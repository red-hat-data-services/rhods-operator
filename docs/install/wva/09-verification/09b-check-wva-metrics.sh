#!/bin/bash
# Verify that the Workload Variant Autoscaler (WVA) controller emits its
# metrics to Prometheus: wva_desired_replicas, wva_current_replicas, etc.
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-sim-llama}"
TOKEN=$(oc whoami -t)
THANOS_URL="https://$(oc get route thanos-querier -n openshift-monitoring -o jsonpath='{.spec.host}')"

echo "=== Checking WVA controller metrics ==="

METRICS=(
  "wva_desired_replicas"
  "wva_current_replicas"
  "wva_desired_ratio"
  "wva_replica_scaling_total"
)

found=0
for metric in "${METRICS[@]}"; do
  result=$(curl -sk -H "Authorization: Bearer $TOKEN" \
    "${THANOS_URL}/api/v1/query" \
    --data-urlencode "query=${metric}{exported_namespace=\"${NS}\"}" 2>/dev/null)
  count=$(echo "$result" | python3 -c "import sys,json; r=json.load(sys.stdin); print(len(r.get('data',{}).get('result',[])))" 2>/dev/null || echo "0")
  if [ "$count" -gt 0 ]; then
    value=$(echo "$result" | python3 -c "import sys,json; r=json.load(sys.stdin); print(r['data']['result'][0]['value'][1])" 2>/dev/null || echo "?")
    echo "  FOUND: $metric = $value"
    found=$((found+1))
  else
    echo "  NOT FOUND: $metric"
  fi
done

echo ""
if [ "$found" -gt 0 ]; then
  echo "Found $found WVA metric(s)."
else
  echo "No WVA metrics found."
  echo ""
  echo "Debugging:"
  echo "  1. Check WVA controller is running:"
  echo "     oc get pods -n redhat-ods-applications -l app.kubernetes.io/name=workload-variant-autoscaler"
  echo "  2. Check VariantAutoscaling resource:"
  echo "     oc get variantautoscaling -n $NS"
  echo "  3. Check WVA controller logs:"
  echo "     oc logs deployment/workload-variant-autoscaler-controller-manager -n redhat-ods-applications --tail=50"
fi
