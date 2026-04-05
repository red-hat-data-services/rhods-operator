#!/bin/bash
# Generate traffic through the gateway to produce traces.
# Port-forwards the gateway service on 9443 and sends 13 requests
# (at 10% sample rate, ~75% chance of capturing at least one trace).
set -euo pipefail

NAMESPACE="distributed-tracing"
GATEWAY_SVC="distributed-tracing-gateway-data-science-gateway-class"
LOCAL_PORT=9443
REMOTE_PORT=443
NUM_REQUESTS=13

TOKEN=$(oc whoami -t)

# Start port-forward in the background
echo "==> Port-forwarding $GATEWAY_SVC $LOCAL_PORT:$REMOTE_PORT..."
kubectl port-forward -n "$NAMESPACE" "svc/$GATEWAY_SVC" "$LOCAL_PORT:$REMOTE_PORT" &
PF_PID=$!

# Clean up port-forward on exit
trap 'echo "==> Stopping port-forward (pid $PF_PID)..."; kill $PF_PID 2>/dev/null; wait $PF_PID 2>/dev/null' EXIT

# Wait for port-forward to be ready
sleep 3

echo "==> Sending $NUM_REQUESTS requests..."
for i in $(seq 1 "$NUM_REQUESTS"); do
    HTTP_CODE=$(curl -sk -o /dev/null -w '%{http_code}' \
        --noproxy localhost \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        https://localhost:${LOCAL_PORT}/distributed-tracing/distributed-tracing-llama/v1/chat/completions \
        -d '{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"Request at '"$(date +%s%N)"': Write a detailed essay about topic number '"$i"'. Include historical context, current developments, and future predictions."}],"max_tokens":512}')
    echo "Request $i/$NUM_REQUESTS — HTTP $HTTP_CODE"
done

echo "==> Done. Check the Jaeger UI for traces."
