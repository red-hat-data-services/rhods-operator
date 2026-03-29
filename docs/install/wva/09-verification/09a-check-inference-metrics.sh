#!/bin/bash
# Verify that inference worker (sim or vLLM) metrics are visible in Prometheus.
# These metrics are scraped via the kserve-llm-isvc-vllm-engine PodMonitor.
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"
TOKEN=$(oc whoami -t)
THANOS_URL="https://$(oc get route thanos-querier -n openshift-monitoring -o jsonpath='{.spec.host}')"

echo "=== Checking inference worker metrics for $ISVC in $NS ==="

# The PodMonitor relabels vllm:* to kserve_vllm:* so only the relabeled
# names appear in Prometheus.
QUERIES=(
  'kserve_vllm:num_requests_running{namespace="'"$NS"'"}'
  'kserve_vllm:num_requests_waiting{namespace="'"$NS"'"}'
  'kserve_vllm:kv_cache_usage_perc{namespace="'"$NS"'"}'
)

found=0
for query in "${QUERIES[@]}"; do
  result=$(curl -sk -G -H "Authorization: Bearer $TOKEN" \
    "${THANOS_URL}/api/v1/query" --data-urlencode "query=${query}" 2>/dev/null)
  count=$(echo "$result" | python3 -c "import sys,json; r=json.load(sys.stdin); print(len(r.get('data',{}).get('result',[])))" 2>/dev/null || echo "0")
  if [ "$count" -gt 0 ]; then
    value=$(echo "$result" | python3 -c "import sys,json; r=json.load(sys.stdin); print(r['data']['result'][0]['value'][1])" 2>/dev/null || echo "?")
    echo "  FOUND: $query = $value"
    found=$((found+1))
  else
    echo "  NOT FOUND: $query"
  fi
done

echo ""
if [ "$found" -gt 0 ]; then
  echo "Found $found inference worker metric(s) in Prometheus."
else
  echo "No inference worker metrics found in Prometheus yet."
  echo "It may take a few minutes for scraping to begin."
  echo ""
  echo "Checking raw metrics directly from the inference worker pod..."
  pod=$(oc get pod -n "$NS" -l "app.kubernetes.io/name=$ISVC,app.kubernetes.io/component=llminferenceservice-workload" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -n "$pod" ]; then
    echo "  Pod: $pod"
    echo "  Raw metrics endpoint (first 20 lines):"
    oc exec "$pod" -n "$NS" -- curl -sk https://localhost:8000/metrics 2>/dev/null | head -20
  fi
fi
