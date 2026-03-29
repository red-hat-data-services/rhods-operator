#!/bin/bash
# Scale down non-essential components to free CPU on resource-constrained clusters.
# The ODH operator may scale ODH components back up on reconciliation.
set -euo pipefail

echo "Scaling down non-essential ODH deployments..."
for deploy in \
  notebook-controller-deployment \
  odh-notebook-controller-manager \
  odh-model-controller \
  dashboard-redirect; do
  if kubectl get deployment "$deploy" -n redhat-ods-applications &>/dev/null; then
    kubectl scale deployment "$deploy" -n redhat-ods-applications --replicas=0
    echo "  Scaled down redhat-ods-applications/$deploy"
  fi
done

echo "Scaling down non-essential OpenShift workloads..."
kubectl scale deployment downloads -n openshift-console --replicas=0
echo "  Scaled down openshift-console/downloads"
kubectl scale deployment cluster-samples-operator -n openshift-cluster-samples-operator --replicas=0
echo "  Scaled down openshift-cluster-samples-operator/cluster-samples-operator"
kubectl scale statefulset alertmanager-main -n openshift-monitoring --replicas=0
echo "  Scaled down openshift-monitoring/alertmanager-main"
kubectl scale deployment rhods-operator -n redhat-ods-operator --replicas=1
echo "  Scaled rhods-operator to 1 replica (from 3)"

echo "Done."
