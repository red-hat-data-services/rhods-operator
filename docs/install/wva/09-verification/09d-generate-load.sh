#!/bin/bash
# Generate sustained load against the LLMInferenceService gateway via port-forward.
# Usage: ./09d-generate-load.sh [namespace] [isvc-name] [concurrency] [duration-seconds] [local-port]
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"
CONCURRENCY="${3:-200}"
DURATION="${4:-300}"
LOCAL_PORT="${5:-9443}"

# Get the gateway service name and route path from the LLMISVC
GATEWAY_SVC=$(oc get svc -n "$NS" -l gateway.networking.k8s.io/gateway-name -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$GATEWAY_SVC" ]; then
  echo "ERROR: Could not find gateway service in namespace $NS"
  exit 1
fi

ROUTE_PATH=$(oc get llminferenceservice "$ISVC" -n "$NS" -o jsonpath='{.status.addresses[0].url}' 2>/dev/null \
  | sed 's|https://[^/]*/|/|')
if [ -z "$ROUTE_PATH" ]; then
  ROUTE_PATH="/${NS}/${ISVC}"
fi

# Start port-forward
echo "Starting port-forward to $GATEWAY_SVC on localhost:$LOCAL_PORT..."
oc port-forward "svc/$GATEWAY_SVC" -n "$NS" "$LOCAL_PORT:443" &>/dev/null &
PF_PID=$!
sleep 2

COMPLETIONS_URL="https://localhost:${LOCAL_PORT}${ROUTE_PATH}/v1/chat/completions"
TOKEN=$(oc whoami -t)

# Use unique prompts per request to defeat vLLM prefix cache (otherwise cached
# KV blocks make responses instant and no queue depth builds up for scaling).
make_body() {
  local seq=$1
  cat <<JSON
{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"Request ${seq} at $(date +%s%N): Write a detailed essay about topic number ${seq}. Include historical context, current developments, and future predictions."}],"max_tokens":512}
JSON
}

echo "=== Load Generator ==="
echo "Target:      $COMPLETIONS_URL"
echo "Concurrency: $CONCURRENCY"
echo "Duration:    ${DURATION}s"
echo ""

RESULTS_DIR=$(mktemp -d)
trap 'kill $PF_PID 2>/dev/null; wait $PF_PID 2>/dev/null; SENT=$(find "$RESULTS_DIR" -name "ok_*" 2>/dev/null | wc -l | tr -d " "); ERRORS=$(find "$RESULTS_DIR" -name "err_*" 2>/dev/null | wc -l | tr -d " "); echo ""; echo "=== Summary ==="; echo "Sent: $SENT requests ($ERRORS errors) in ${SECONDS}s"; rm -rf "$RESULTS_DIR"; echo "Port-forward stopped."' EXIT INT TERM

END=$((SECONDS + DURATION))
SEQ=0
ACTIVE=0

# Fire off requests continuously, maintaining CONCURRENCY in-flight at all times.
# Instead of batch-then-wait, we launch new requests as fast as possible and only
# throttle when we hit the concurrency cap.
while [ $SECONDS -lt $END ]; do
  SEQ=$((SEQ + 1))
  ( curl -sk --noproxy localhost --max-time 30 "$COMPLETIONS_URL" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$(make_body $SEQ)" \
      -o /dev/null -w "" \
    && touch "$RESULTS_DIR/ok_${SEQ}" \
    || touch "$RESULTS_DIR/err_${SEQ}" ) &
  ACTIVE=$((ACTIVE + 1))

  # When we hit the concurrency cap, wait for the current batch to drain
  # and report progress.
  if (( ACTIVE >= CONCURRENCY )); then
    wait
    ACTIVE=0
    SENT=$(find "$RESULTS_DIR" -name "ok_*" 2>/dev/null | wc -l | tr -d " ")
    ERRORS=$(find "$RESULTS_DIR" -name "err_*" 2>/dev/null | wc -l | tr -d " ")
    echo "[$(date +%H:%M:%S)] Completed $SENT requests ($ERRORS errors) | $(( END - SECONDS ))s remaining"
  fi
done
wait
