#!/bin/bash
# Generate sustained load against the LLMInferenceService gateway.
# Usage: ./09d-generate-load.sh [namespace] [isvc-name] [concurrency] [duration-seconds]
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"
CONCURRENCY="${3:-10}"
DURATION="${4:-120}"

GATEWAY_URL=$(oc get llminferenceservice "$ISVC" -n "$NS" -o jsonpath='{.status.url}')
if [ -z "$GATEWAY_URL" ]; then
  echo "ERROR: Could not get gateway URL from llminferenceservice $ISVC"
  exit 1
fi

COMPLETIONS_URL="${GATEWAY_URL}/v1/chat/completions"
BODY='{"model":"unsloth/Meta-Llama-3.1-8B","messages":[{"role":"user","content":"Tell me a long story"}],"max_tokens":100}'

echo "=== Load Generator ==="
echo "Target:      $COMPLETIONS_URL"
echo "Concurrency: $CONCURRENCY"
echo "Duration:    ${DURATION}s"
echo ""

RESULTS_DIR=$(mktemp -d)
trap 'SENT=$(find "$RESULTS_DIR" -name "ok_*" 2>/dev/null | wc -l | tr -d " "); ERRORS=$(find "$RESULTS_DIR" -name "err_*" 2>/dev/null | wc -l | tr -d " "); echo ""; echo "=== Summary ==="; echo "Sent: $SENT requests ($ERRORS errors) in ${SECONDS}s"; rm -rf "$RESULTS_DIR"' EXIT

END=$((SECONDS + DURATION))
SEQ=0

while [ $SECONDS -lt $END ]; do
  for _ in $(seq 1 "$CONCURRENCY"); do
    SEQ=$((SEQ + 1))
    ( curl -sk "$COMPLETIONS_URL" \
        -H "Content-Type: application/json" \
        -d "$BODY" \
        -o /dev/null -w "" \
      && touch "$RESULTS_DIR/ok_${SEQ}" \
      || touch "$RESULTS_DIR/err_${SEQ}" ) &
  done
  wait
  SENT=$(find "$RESULTS_DIR" -name "ok_*" 2>/dev/null | wc -l | tr -d " ")
  ERRORS=$(find "$RESULTS_DIR" -name "err_*" 2>/dev/null | wc -l | tr -d " ")
  echo "[$(date +%H:%M:%S)] Sent $SENT requests ($ERRORS errors) | $(( END - SECONDS ))s remaining"
done
wait
