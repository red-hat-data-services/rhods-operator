#!/bin/bash
# Stands up the full WVA stack on an OpenShift cluster.
# Usage: ./standup.sh
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

info()  { echo "==> $*"; }
wait_for_pods() {
  local ns="$1" label="$2" count="$3" timeout="${4:-300}"
  info "Waiting for $count pod(s) with label '$label' in $ns (timeout ${timeout}s)..."
  local elapsed=0
  while true; do
    ready=$(kubectl get pods -n "$ns" -l "$label" --no-headers 2>/dev/null \
      | grep -c 'Running' || true)
    if [ "$ready" -ge "$count" ]; then
      info "$ready/$count pod(s) running"
      return 0
    fi
    if [ "$elapsed" -ge "$timeout" ]; then
      echo "ERROR: timed out waiting for pods (label=$label, ns=$ns)"
      kubectl get pods -n "$ns" -l "$label" 2>/dev/null || true
      exit 1
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done
}

wait_for_crd() {
  local crd="$1" timeout="${2:-300}"
  info "Waiting for CRD '$crd' (timeout ${timeout}s)..."
  local elapsed=0
  while true; do
    if kubectl get crd "$crd" &>/dev/null; then
      info "CRD $crd exists"
      return 0
    fi
    if [ "$elapsed" -ge "$timeout" ]; then
      echo "ERROR: timed out waiting for CRD $crd"
      exit 1
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done
}

# ── Step 1: Prerequisites (parallel) ─────────────────────────────────────────
# info "Step 1: Creating UWM config, Custom Metrics Autoscaler, and Connectivity Link..."
# kubectl apply -f "$DIR/00-uwm-cm.yaml"
# kubectl apply -f "$DIR/01-custom-metrics-autoscaler.yaml"
# kubectl apply -f "$DIR/02-connectivity-link.yaml"

# # ── Step 2: Wait for KEDA operator ───────────────────────────────────────────
# info "Step 2: Waiting for KEDA operator..."
# wait_for_crd "kedacontrollers.keda.sh"
# wait_for_pods "openshift-keda" "name=custom-metrics-autoscaler-operator" 1

# # ── Step 3: Patch KedaController ─────────────────────────────────────────────
# info "Step 3: Applying KedaController (watchNamespace='')..."
# kubectl apply -f "$DIR/03-keda-controller.yaml"

# # ── Step 4: Deploy RHODS operator ────────────────────────────────────────────
# info "Step 4: Deploying RHODS operator..."
# bash "$DIR/04-rhods-operator/04-deploy-and-patch-odh-operator-with-sa.sh"

# # ── Step 5: Wait for RHODS operator pods ─────────────────────────────────────
# info "Step 5: Waiting for RHODS operator pods..."
# wait_for_pods "redhat-ods-operator" "name=rhods-operator" 1

# # ── Step 6: Create DSCI and DSC ──────────────────────────────────────────────
# info "Step 6: Waiting for RHODS webhook to become ready..."
# timeout=120; elapsed=0
# while ! kubectl get endpoints rhods-operator-webhook-service -n redhat-ods-operator -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null | grep -q .; do
#   if [ "$elapsed" -ge "$timeout" ]; then
#     echo "ERROR: timed out waiting for RHODS webhook endpoint"
#     exit 1
#   fi
#   sleep 5
#   elapsed=$((elapsed + 5))
# done
# info "RHODS webhook is ready. Creating DSC..."
# kubectl apply -f "$DIR/05-wva-dsc.yaml"

# # Patch component SAs with pull secret as soon as they exist (before DSC is fully ready).
# # The DSC reconciliation creates the SAs and then the deployments — we need to patch
# # the SAs before the pods start pulling images.
# info "Waiting for component SAs to exist and patching with pull secret..."
# for sa in kserve-controller-manager llmisvc-controller-manager workload-variant-autoscaler-controller-manager; do
#     timeout=120; elapsed=0
#     while ! kubectl get sa "$sa" -n redhat-ods-applications &>/dev/null; do
#         if [ "$elapsed" -ge "$timeout" ]; then
#             echo "ERROR: timed out waiting for SA $sa"
#             exit 1
#         fi
#         sleep 3
#         elapsed=$((elapsed + 3))
#     done
#     oc patch sa "$sa" -n redhat-ods-applications \
#         -p '{"imagePullSecrets": [{"name": "rhoai-operator-pull-secret"}]}' --type=merge
# done

# # Delete pods immediately so they restart with the patched SAs.
# # If we wait for DSC Ready first, the pods stay in ImagePullBackOff and the DSC never becomes Ready.
# info "Restarting component controller pods to pick up pull secret..."
# kubectl delete pods -l control-plane=kserve-controller-manager -n redhat-ods-applications --ignore-not-found
# kubectl delete pods -l control-plane=llmisvc-controller-manager -n redhat-ods-applications --ignore-not-found
# kubectl delete pods -l app.kubernetes.io/name=workload-variant-autoscaler -n redhat-ods-applications --ignore-not-found

# info "Waiting for DSC 'default-dsc' to be ready (timeout 300s)..."
# timeout=300; elapsed=0
# while true; do
#   phase=$(kubectl get datasciencecluster default-dsc -o jsonpath='{.status.phase}' 2>/dev/null || true)
#   if [ "$phase" = "Ready" ]; then
#     info "DSC default-dsc is Ready"
#     break
#   fi
#   if [ "$elapsed" -ge "$timeout" ]; then
#     echo "ERROR: timed out waiting for DSC default-dsc to be ready (current phase: $phase)"
#     kubectl get datasciencecluster default-dsc -o yaml 2>/dev/null || true
#     exit 1
#   fi
#   sleep 10
#   elapsed=$((elapsed + 10))
# done

# # ── Step 7: Scale down non-essential components ──────────────────────────────
# info "Step 7: Scaling down non-essential components..."
# bash "$DIR/06-scale-down-non-essential.sh"

# # ── Step 8: KEDA auth setup ──────────────────────────────────────────────────
# info "Step 8: Setting up KEDA auth for Thanos..."
# kubectl apply -f "$DIR/07-auth/07a-service-account.yaml"
# kubectl apply -f "$DIR/07-auth/07b-cluster-role-binding.yaml"
# kubectl apply -f "$DIR/07-auth/07c-trigger-authentication.yaml"
# info "Waiting for inferenceservice-config to exist..."
# timeout=120; elapsed=0
# while ! kubectl get configmap inferenceservice-config -n redhat-ods-applications &>/dev/null; do
#   if [ "$elapsed" -ge "$timeout" ]; then
#     echo "ERROR: timed out waiting for inferenceservice-config ConfigMap"
#     exit 1
#   fi
#   sleep 5
#   elapsed=$((elapsed + 5))
# done
# cd "$DIR/07-auth" && bash 07d-run-patch.sh && cd "$DIR"

# # ── Step 9: Deploy LLMInferenceService ───────────────────────────────────────
# info "Step 9: Waiting for llmisvc webhook to become ready..."
# timeout=120; elapsed=0
# while ! kubectl get endpoints llmisvc-webhook-server-service -n redhat-ods-applications -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null | grep -q .; do
#   if [ "$elapsed" -ge "$timeout" ]; then
#     echo "ERROR: timed out waiting for llmisvc webhook endpoint"
#     exit 1
#   fi
#   sleep 5
#   elapsed=$((elapsed + 5))
# done
# info "llmisvc webhook is ready. Deploying LLMInferenceService..."
bash "$DIR/08-llimsvc/apply.sh"

info "Waiting for LLMInferenceService sim-llama to be ready..."
timeout=600; elapsed=0
while true; do
  ready=$(kubectl get llminferenceservice sim-llama -n autoscaling-example -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
  if [ "$ready" = "True" ]; then
    info "LLMInferenceService sim-llama is ready"
    break
  fi
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "ERROR: timed out waiting for LLMInferenceService sim-llama to be ready"
    kubectl get llminferenceservice sim-llama -n autoscaling-example -o yaml 2>/dev/null || true
    exit 1
  fi
  sleep 10
  elapsed=$((elapsed + 10))
done

info "Stack is up! Run the verification scripts in 09-verification/ to check."
