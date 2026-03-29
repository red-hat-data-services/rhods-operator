#!/bin/bash
# Test autoscaling by sending a burst of requests through the gateway,
# then watching the deployment scale up and back down.
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"

GATEWAY_URL=$(oc get llminferenceservice "$ISVC" -n "$NS" -o jsonpath='{.status.url}')
if [ -z "$GATEWAY_URL" ]; then
  echo "ERROR: Could not get gateway URL from llminferenceservice $ISVC"
  exit 1
fi

COMPLETIONS_URL="${GATEWAY_URL}/v1/chat/completions"

echo "=== Autoscaling Test ==="
echo "Gateway URL: $COMPLETIONS_URL"
echo ""

# Show initial state
echo "--- Initial State ---"
oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='Replicas: {.spec.replicas}/{.status.readyReplicas} ready' && echo ""
oc get scaledobject -n "$NS" --no-headers 2>/dev/null
oc get hpa -n "$NS" --no-headers 2>/dev/null
echo ""

# Send burst of concurrent requests
CONCURRENCY="${3:-20}"
REQUESTS="${4:-100}"
echo "--- Sending $REQUESTS requests ($CONCURRENCY concurrent) ---"
echo "This will take a moment..."

for i in $(seq 1 "$REQUESTS"); do
  curl -sk "$COMPLETIONS_URL" \
    -H "Content-Type: application/json" \
    -d '{"model": "unsloth/Meta-Llama-3.1-8B", "messages": [{"role": "user", "content": "Tell me a long story about the history of computing from the 1950s to present day in great detail"}], "max_tokens": 100}' \
    -o /dev/null -w "" &

  # Limit concurrency
  if (( i % CONCURRENCY == 0 )); then
    wait
    echo "  Sent $i/$REQUESTS requests..."
  fi
done
wait
echo "  All $REQUESTS requests sent."
echo ""

# Watch scaling
echo "--- Watching scale events (Ctrl+C to stop) ---"
echo "Checking every 5 seconds for 2 minutes..."
for i in $(seq 1 24); do
  replicas=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.spec.replicas}' 2>/dev/null)
  ready=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  hpa_target=$(oc get hpa -n "$NS" --no-headers 2>/dev/null | awk '{print $3}')
  echo "  [$(date +%H:%M:%S)] Replicas: ${replicas} (${ready:-0} ready) | HPA targets: ${hpa_target:-n/a}"
  if [ "$replicas" -gt 1 ]; then
    echo "  ** Scale-up detected! **"
  fi
  sleep 5
done
