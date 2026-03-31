#!/bin/bash
# Test WVA autoscaling by running a load generator pod inside the cluster
# and watching the deployment scale up.
#
# Usage:
#   ./09c-test-autoscaling.sh [namespace] [isvc-name] [concurrency] [requests]
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"
CONCURRENCY="${3:-200}"
REQUESTS="${4:-5000}"
POD_NAME="load-test-$(date +%s)"
CM_NAME="script-${POD_NAME}"

# Resolve gateway service FQDN
GATEWAY_SVC=$(oc get svc -n "$NS" -l gateway.networking.k8s.io/gateway-name \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$GATEWAY_SVC" ]; then
  echo "ERROR: No gateway service found in namespace $NS"
  exit 1
fi
URL="https://${GATEWAY_SVC}.${NS}.svc.cluster.local/${NS}/${ISVC}/v1/chat/completions"

echo "=== WVA Autoscaling Test ==="
echo "URL:         $URL"
echo "Concurrency: $CONCURRENCY"
echo "Requests:    $REQUESTS"
echo ""
oc get deployment "${ISVC}-kserve" -n "$NS" \
  -o jsonpath='Initial state: {.spec.replicas}/{.status.readyReplicas} ready' && echo ""
echo ""

# Cleanup on exit
cleanup() {
  echo ""
  echo "Cleaning up..."
  oc delete pod "$POD_NAME" -n "$NS" --ignore-not-found --wait=false 2>/dev/null
  oc delete configmap "$CM_NAME" -n "$NS" --ignore-not-found 2>/dev/null
}
trap cleanup EXIT INT TERM

# Load script that runs inside the cluster pod.
# Uses a wrapper script so xargs passes arguments cleanly.
read -r -d '' LOAD_SCRIPT << 'EOF' || true
#!/bin/sh
set -e
URL="$1"; CONCURRENCY="$2"; REQUESTS="$3"

# Wrapper script for xargs — each invocation sends one request
cat > /tmp/req.sh << 'REQEOF'
#!/bin/sh
curl -sk --max-time 600 "$1" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"Qwen/Qwen2.5-7B-Instruct\",\"messages\":[{\"role\":\"user\",\"content\":\"Request $2. Write a detailed essay about topic $2 covering history, analysis, and predictions.\"}],\"max_tokens\":2048}" \
  -o /dev/null -w "req=$2 status=%{http_code} time=%{time_total}s\n"
REQEOF
chmod +x /tmp/req.sh

# Verify connectivity
echo "Smoke test..."
STATUS=$(curl -sk --max-time 30 "$URL" \
  -H "Content-Type: application/json" \
  -d '{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"Hi"}],"max_tokens":5}' \
  -o /dev/null -w "%{http_code}")
echo "Status: $STATUS"
if [ "$STATUS" != "200" ]; then
  echo "ERROR: Smoke test failed (HTTP $STATUS)"
  exit 1
fi

echo "Sending $REQUESTS requests ($CONCURRENCY concurrent)..."
seq 1 "$REQUESTS" | xargs -P "$CONCURRENCY" -I{} /tmp/req.sh "$URL" {}
echo "Done."
EOF

# Deploy load generator
oc create configmap "$CM_NAME" -n "$NS" --from-literal=load.sh="$LOAD_SCRIPT"

cat <<MANIFEST | oc apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME
  namespace: $NS
spec:
  restartPolicy: Never
  containers:
  - name: load
    image: curlimages/curl
    command: ["sh", "/scripts/load.sh"]
    args: ["$URL", "$CONCURRENCY", "$REQUESTS"]
    resources:
      requests: { cpu: "4", memory: "4Gi" }
      limits:   { cpu: "8", memory: "8Gi" }
    volumeMounts:
    - { name: script, mountPath: /scripts }
  volumes:
  - name: script
    configMap: { name: $CM_NAME, defaultMode: 0755 }
MANIFEST

echo "Waiting for pod..."
oc wait --for=condition=Ready pod/"$POD_NAME" -n "$NS" --timeout=120s 2>/dev/null || true
sleep 2

POD_PHASE=$(oc get pod "$POD_NAME" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null)
if [ "$POD_PHASE" = "Failed" ]; then
  echo "ERROR: Pod failed:"
  oc logs "$POD_NAME" -n "$NS" 2>/dev/null
  exit 1
fi

# Stream logs in background, watch replicas in foreground
oc logs -f "$POD_NAME" -n "$NS" 2>/dev/null &
LOGS_PID=$!

echo ""
echo "--- Watching replicas (Ctrl+C to stop) ---"
for _ in $(seq 1 120); do
  replicas=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.spec.replicas}' 2>/dev/null)
  ready=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  echo "  [$(date +%H:%M:%S)] Replicas: ${replicas:-?} (${ready:-0} ready)"
  [ "${replicas:-1}" -gt 1 ] && echo "  ** Scale-up detected! **"
  oc get pod "$POD_NAME" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null \
    | grep -q "Succeeded\|Failed" && echo "  Load pod finished." && break
  sleep 5
done

kill "$LOGS_PID" 2>/dev/null || true
wait "$LOGS_PID" 2>/dev/null || true
