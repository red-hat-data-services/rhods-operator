#!/bin/bash
# Tears down the full WVA stack from an OpenShift cluster.
# Usage: ./teardown.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

info() { echo "==> $*"; }
warn() { echo "  WARN: $*"; }

# Wrapper: attempt delete, warn on failure but continue.
safe_delete() {
  if ! kubectl delete "$@" --ignore-not-found 2>&1; then
    warn "failed to delete ($*) — may require manual cleanup"
  fi
}

# ── Step 1: Delete LLMInferenceService and namespace ─────────────────────────
info "Step 1: Deleting LLMInferenceService resources..."
safe_delete -f "$DIR/08-llimsvc/08c-llmisvc.yaml"
safe_delete -f "$DIR/08-llimsvc/08b-gateway.yaml"
safe_delete -f "$DIR/08-llimsvc/08a-namespace.yaml"

# ── Step 2: Delete KEDA auth resources ───────────────────────────────────────
info "Step 2: Deleting KEDA auth resources..."
safe_delete -f "$DIR/07-auth/07c-trigger-authentication.yaml"
safe_delete -f "$DIR/07-auth/07b-cluster-role-binding.yaml"
safe_delete -f "$DIR/07-auth/07a-service-account.yaml"

# ── Step 3: Delete DSC and DSCI ──────────────────────────────────────────────
info "Step 3: Deleting DSC..."
safe_delete -f "$DIR/05-wva-dsc.yaml"

# ── Step 4: Undeploy RHODS operator ──────────────────────────────────────────
info "Step 4: Undeploying RHODS operator..."
bash "$DIR/04-rhods-operator/04-teardown.sh" || warn "04-teardown.sh failed"

# ── Step 5: Delete KedaController ────────────────────────────────────────────
info "Step 5: Deleting KedaController..."
safe_delete -f "$DIR/03-keda-controller.yaml"

# ── Step 6: Delete operators and UWM config ──────────────────────────────────
info "Step 6: Deleting operators and UWM config..."
safe_delete -f "$DIR/02-connectivity-link.yaml"
safe_delete -f "$DIR/01-custom-metrics-autoscaler.yaml"
safe_delete -f "$DIR/00-uwm-cm.yaml"

info "Teardown complete."
