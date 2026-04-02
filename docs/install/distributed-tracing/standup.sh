#!/bin/bash
# Stands up the distributed tracing demo stack on an OpenShift cluster.
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

# ── Step 0: Create namespace ────────────────────────────────────────────────
info "Step 0: Creating namespace..."
kubectl apply -f "$DIR/00-namespace.yaml"

# ── Step 1: Install OpenTelemetry operator ──────────────────────────────────
info "Step 1: Installing OpenTelemetry operator..."
bash "$DIR/01-otel/apply.sh"

# ── Step 2: Install Connectivity Link operator ──────────────────────────────
info "Step 2: Installing Connectivity Link operator..."
bash "$DIR/02-connectivity-link/apply.sh"

# ── Step 3: Install Tempo operator, instance, and OTel collector ────────────
info "Step 3: Installing Tempo operator, instance, and OTel collector..."
bash "$DIR/03-tempo/apply.sh"

# ── Step 4: Deploy RHODS operator ────────────────────────────────────────────
info "Step 4: Deploying RHODS operator..."
bash "$DIR/04-rhods-operator/04-deploy-and-patch-odh-operator-with-sa.sh"

# ── Step 5: Wait for RHODS operator pods ─────────────────────────────────────
info "Step 5: Waiting for RHODS operator pods..."
wait_for_pods "redhat-ods-operator" "name=rhods-operator" 1

info "Waiting for RHODS webhook to become ready..."
timeout=120; elapsed=0
while ! kubectl get endpoints rhods-operator-webhook-service -n redhat-ods-operator -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null | grep -q .; do
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "ERROR: timed out waiting for RHODS webhook endpoint"
    exit 1
  fi
  sleep 5
  elapsed=$((elapsed + 5))
done
info "RHODS webhook is ready."

# ── Step 5b: Create DSC ─────────────────────────────────────────────────────
info "Step 5b: Applying DSC..."
bash "$DIR/05-dsc/apply.sh"

info "Waiting for DSC 'default-dsc' to be ready (timeout 300s)..."
timeout=300; elapsed=0
while true; do
  phase=$(kubectl get datasciencecluster default-dsc -o jsonpath='{.status.phase}' 2>/dev/null || true)
  if [ "$phase" = "Ready" ]; then
    info "DSC default-dsc is Ready"
    break
  fi
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "ERROR: timed out waiting for DSC default-dsc to be ready (current phase: $phase)"
    kubectl get datasciencecluster default-dsc -o yaml 2>/dev/null || true
    exit 1
  fi
  sleep 10
  elapsed=$((elapsed + 10))
done

# ── Step 6: Scale down non-essential components ──────────────────────────────
info "Step 6: Scaling down non-essential components..."
bash "$DIR/06-scale-down-non-essential.sh"

# ── Step 7: Deploy LLMInferenceService ───────────────────────────────────────
info "Step 7: Waiting for llmisvc webhook to become ready..."
timeout=120; elapsed=0
while ! kubectl get endpoints llmisvc-webhook-server-service -n redhat-ods-applications -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null | grep -q .; do
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "ERROR: timed out waiting for llmisvc webhook endpoint"
    exit 1
  fi
  sleep 5
  elapsed=$((elapsed + 5))
done
info "llmisvc webhook is ready. Deploying LLMInferenceService..."
bash "$DIR/07-llmisvc/apply.sh"

info "Waiting for LLMInferenceService distributed-tracing-llama to be ready..."
timeout=600; elapsed=0
while true; do
  ready=$(kubectl get llminferenceservice distributed-tracing-llama -n distributed-tracing -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
  if [ "$ready" = "True" ]; then
    info "LLMInferenceService distributed-tracing-llama is ready"
    break
  fi
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "ERROR: timed out waiting for LLMInferenceService distributed-tracing-llama to be ready"
    kubectl get llminferenceservice distributed-tracing-llama -n distributed-tracing -o yaml 2>/dev/null || true
    exit 1
  fi
  sleep 10
  elapsed=$((elapsed + 10))
done

info "Stack is up! Run 08-verification/generate-traffic.sh to generate traces, then check the Jaeger UI."
