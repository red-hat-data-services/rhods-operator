#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

# Namespace is created in 00-namespace.yaml (applied earlier in the standup flow).

# Create pull secret in the workload namespace for scheduler/sidecar images
kubectl create secret docker-registry rhoai-operator-pull-secret -n distributed-tracing \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f "$DIR/07a-gateway.yaml"
kubectl apply -f "$DIR/07b-llmisvc.yaml"

# The llmisvc controller creates the SA after the LLMInferenceService is applied.
# Wait for it, then patch it with the pull secret and restart the pods.
info() { echo "==> $*"; }
info "Waiting for SA distributed-tracing-llama-epp-sa to exist..."
timeout=120; elapsed=0
while ! kubectl get sa distributed-tracing-llama-epp-sa -n distributed-tracing &>/dev/null; do
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: timed out waiting for SA distributed-tracing-llama-epp-sa"
        exit 1
    fi
    sleep 3
    elapsed=$((elapsed + 3))
done
oc patch sa distributed-tracing-llama-epp-sa -n distributed-tracing \
    -p '{"imagePullSecrets": [{"name": "rhoai-operator-pull-secret"}]}' --type=merge
sleep 10
kubectl delete pods -l app.kubernetes.io/component=llminferenceservice-workload -n distributed-tracing --ignore-not-found
kubectl delete pods -l kubernetes.io/component=llminferenceservice-router-scheduler -n distributed-tracing --ignore-not-found
