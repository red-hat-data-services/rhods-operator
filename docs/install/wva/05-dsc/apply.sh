#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Applying DSC..."
kubectl apply -f "$DIR/05a-wva-dsc.yaml"

oc create secret docker-registry rhoai-operator-pull-secret -n redhat-ods-applications \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json \
    --dry-run=client -o yaml | oc apply -f -

# Wait for the component SAs to exist, then patch them with the pull secret
# so the controller pods can pull images from authenticated registries.
echo "Waiting for component SAs to exist and patching with pull secret..."
for sa in kserve-controller-manager llmisvc-controller-manager workload-variant-autoscaler-controller-manager; do
    timeout=120; elapsed=0
    while ! kubectl get sa "$sa" -n redhat-ods-applications &>/dev/null; do
        if [ "$elapsed" -ge "$timeout" ]; then
            echo "ERROR: timed out waiting for SA $sa"
            exit 1
        fi
        sleep 3
        elapsed=$((elapsed + 3))
    done
    oc patch sa "$sa" -n redhat-ods-applications \
        -p '{"imagePullSecrets": [{"name": "rhoai-operator-pull-secret"}]}' --type=merge
done

# Delete controller pods so they restart with the patched SAs.
# Without this, pods that started before the SA patch stay in ImagePullBackOff.
echo "Restarting component controller pods to pick up pull secret..."
kubectl delete pods -l control-plane=kserve-controller-manager -n redhat-ods-applications --ignore-not-found
kubectl delete pods -l control-plane=llmisvc-controller-manager -n redhat-ods-applications --ignore-not-found
kubectl delete pods -l app.kubernetes.io/name=workload-variant-autoscaler -n redhat-ods-applications --ignore-not-found

# Wait for the 3 controller pods to come back up.
echo "Waiting for controller pods to be running..."
wait_for_pod() {
    local label="$1" timeout=180 elapsed=0
    while true; do
        ready=$(kubectl get pods -n redhat-ods-applications -l "$label" --no-headers 2>/dev/null \
            | grep -c 'Running' || true)
        if [ "$ready" -ge 1 ]; then
            echo "  Pod with label '$label' is running"
            return 0
        fi
        if [ "$elapsed" -ge "$timeout" ]; then
            echo "ERROR: timed out waiting for pod with label '$label'"
            kubectl get pods -n redhat-ods-applications -l "$label" 2>/dev/null || true
            exit 1
        fi
        sleep 5
        elapsed=$((elapsed + 5))
    done
}
wait_for_pod "control-plane=kserve-controller-manager"
wait_for_pod "control-plane=llmisvc-controller-manager"
wait_for_pod "app.kubernetes.io/name=workload-variant-autoscaler"
echo "All controller pods are running."
