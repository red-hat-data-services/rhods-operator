#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

kubectl apply -f "$DIR/08a-namespace.yaml"

# Create pull secret in the workload namespace for scheduler/sidecar images
kubectl create secret docker-registry rhoai-operator-pull-secret -n autoscaling-example \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f "$DIR/08b-gateway.yaml"
kubectl apply -f "$DIR/08c-llmisvc.yaml"

# The llmisvc controller creates the SA after the LLMInferenceService is applied.
# Wait for it, then patch it with the pull secret and restart the pods.
info() { echo "==> $*"; }
info "Waiting for SA autoscaling-example-llama-epp-sa to exist..."
timeout=120; elapsed=0
while ! kubectl get sa autoscaling-example-llama-epp-sa -n autoscaling-example &>/dev/null; do
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: timed out waiting for SA autoscaling-example-llama-epp-sa"
        exit 1
    fi
    sleep 3
    elapsed=$((elapsed + 3))
done
oc patch sa autoscaling-example-llama-epp-sa -n autoscaling-example \
    -p '{"imagePullSecrets": [{"name": "rhoai-operator-pull-secret"}]}' --type=merge
sleep 10
kubectl delete pods -l app.kubernetes.io/component=llminferenceservice-workload-n autoscaling-example --ignore-not-found
kubectl delete pods -l kubernetes.io/component=llminferenceservice-router-scheduler -n autoscaling-example --ignore-not-found

# Create recording rules that alias kserve_vllm:* metrics to vllm:* so WVA
# can find them. The LLMInferenceService PodMonitor relabels vllm:* to
# kserve_vllm:*, but WVA queries for the unprefixed names.
# kubectl apply -f "$DIR/08d-vllm-metrics-recording-rule.yaml"
# echo "Applied vllm metrics recording rule"
