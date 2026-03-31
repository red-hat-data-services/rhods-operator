#!/bin/bash
# Test autoscaling by sending a burst of requests to the gateway, then watching
# the deployment scale up and back down.
#
# Runs the load generator inside the cluster as a Pod to avoid port-forward
# bottlenecks, while controlling everything from your desktop.
#
# Usage:
#   ./09c-test-autoscaling.sh [namespace] [isvc-name] [concurrency] [requests]
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"
CONCURRENCY="${3:-200}"
REQUESTS="${4:-50000}"
POD_NAME="load-test-$(date +%s)"

TOKEN=$(oc whoami -t)

# Resolve gateway service FQDN inside the cluster
GATEWAY_SVC=$(oc get svc -n "$NS" -l gateway.networking.k8s.io/gateway-name -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$GATEWAY_SVC" ]; then
  echo "ERROR: Could not find gateway service in namespace $NS"
  exit 1
fi
GATEWAY_URL="https://${GATEWAY_SVC}.${NS}.svc.cluster.local"
ROUTE_PATH="/${NS}/${ISVC}"
COMPLETIONS_URL="${GATEWAY_URL}${ROUTE_PATH}/v1/chat/completions"

echo "=== Autoscaling Test (in-cluster) ==="
echo "Gateway URL: $COMPLETIONS_URL"
echo "Concurrency: $CONCURRENCY"
echo "Requests:    $REQUESTS"
echo "Pod:         $POD_NAME"
echo ""

# Show initial state
echo "--- Initial State ---"
oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='Replicas: {.spec.replicas}/{.status.readyReplicas} ready' && echo ""
oc get hpa -n "$NS" --no-headers 2>/dev/null
echo ""

# The load script that runs inside the pod.
# Uses xargs for parallel execution instead of shell background jobs, which
# hit process limits in minimal containers.
read -r -d '' LOAD_SCRIPT << 'INNEREOF' || true
#!/bin/sh
set -e


COMPLETIONS_URL="$1"
TOKEN="$2"
CONCURRENCY="$3"
REQUESTS="$4"

echo "Launching $REQUESTS requests at $CONCURRENCY concurrency..."
echo "Target: $COMPLETIONS_URL"

# Write a wrapper script that curls with the token baked in.
# This avoids xargs mangling the token through shell expansion.
cat > /tmp/send-request.sh << CURLEOF
#!/bin/sh
SEQ=\$1
curl -sk --max-time 600 "$COMPLETIONS_URL" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"Qwen/Qwen2.5-7B-Instruct\",\"messages\":[{\"role\":\"user\",\"content\":\"Request \$SEQ. Write a comprehensive and detailed essay exploring topic number \$SEQ. Cover the historical background from earliest origins through modern developments, analyze current state of affairs with specific examples, and provide forward-looking predictions for the next decade.\"}],\"max_tokens\":2048}" \
  -o /dev/null -w "req=\$SEQ status=%{http_code} time=%{time_total}s\n"
CURLEOF
chmod +x /tmp/send-request.sh

echo "--- Debug: send-request.sh contents ---"
cat /tmp/send-request.sh
echo "--- End debug ---"

# Smoke test — single request with full output to verify auth works
echo ""
echo "--- Smoke test ---"
curl -sk --max-time 30 "$COMPLETIONS_URL" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"Hi"}],"max_tokens":5}' \
  -w "\nHTTP status: %{http_code}\n"
echo ""
echo "--- Starting load ---"

# Generate sequence and pipe to xargs for true parallel curl execution.
# Each request gets a unique prompt to defeat vLLM prefix cache.
# The wrapper script has the token baked in so xargs can't corrupt it.
seq 1 "$REQUESTS" | xargs -P "$CONCURRENCY" -I{} /tmp/send-request.sh {}

echo ""
echo "All $REQUESTS requests complete."
INNEREOF

echo "--- Launching load pod in cluster ---"

# Store the load script in a ConfigMap so we can mount it with proper resources
CM_NAME="load-script-${POD_NAME}"
oc create configmap "$CM_NAME" -n "$NS" --from-literal=load.sh="$LOAD_SCRIPT" 2>/dev/null
# Update cleanup to also delete the configmap
cleanup() {
  echo ""
  echo "Cleaning up..."
  oc delete pod "$POD_NAME" -n "$NS" --ignore-not-found --wait=false 2>/dev/null
  oc delete configmap "$CM_NAME" -n "$NS" --ignore-not-found 2>/dev/null
}
trap cleanup EXIT INT TERM

cat <<PODEOF | oc apply -f -
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
    args: ["$COMPLETIONS_URL", "$TOKEN", "$CONCURRENCY", "$REQUESTS"]
    resources:
      requests:
        cpu: "4"
        memory: "4Gi"
      limits:
        cpu: "8"
        memory: "8Gi"
    volumeMounts:
    - name: script
      mountPath: /scripts
  volumes:
  - name: script
    configMap:
      name: $CM_NAME
      defaultMode: 0755
PODEOF

# Wait for pod to start
echo "Waiting for pod to start..."
oc wait --for=condition=Ready pod/"$POD_NAME" -n "$NS" --timeout=120s 2>/dev/null || true
sleep 3

# Check pod status
POD_PHASE=$(oc get pod "$POD_NAME" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null)
echo "Pod phase: $POD_PHASE"
if [ "$POD_PHASE" = "Failed" ]; then
  echo "ERROR: Pod failed to start. Logs:"
  oc logs "$POD_NAME" -n "$NS" 2>/dev/null
  exit 1
fi

# Stream logs in background
echo ""
echo "--- Load generator output ---"
oc logs -f "$POD_NAME" -n "$NS" 2>/dev/null &
LOGS_PID=$!

# Watch scaling in parallel
echo ""
echo "--- Watching scale events (Ctrl+C to stop) ---"
for _ in $(seq 1 60); do
  replicas=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.spec.replicas}' 2>/dev/null)
  ready=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  echo "  [$(date +%H:%M:%S)] Replicas: ${replicas:-?} (${ready:-0} ready)"
  if [ "${replicas:-1}" -gt 1 ]; then
    echo "  ** Scale-up detected! **"
  fi
  # Stop watching if the pod finished
  if oc get pod "$POD_NAME" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null | grep -q "Succeeded\|Failed"; then
    echo "  Load pod finished."
    break
  fi
  sleep 5
done

kill "$LOGS_PID" 2>/dev/null || true
wait "$LOGS_PID" 2>/dev/null || true
