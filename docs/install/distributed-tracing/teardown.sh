#!/bin/bash
# Tears down the distributed tracing demo stack from an OpenShift cluster.
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

# ── Step 1: Delete LLMInferenceService and gateway ──────────────────────────
info "Step 1: Deleting LLMInferenceService resources..."
safe_delete -f "$DIR/07-llmisvc/07b-llmisvc.yaml"
safe_delete -f "$DIR/07-llmisvc/07a-gateway.yaml"

# ── Step 2: Delete DSC ──────────────────────────────────────────────────────
info "Step 2: Deleting DSC..."
safe_delete -f "$DIR/05-dsc/05a-default-dsc.yaml"

# ── Step 3: Undeploy RHODS operator ─────────────────────────────────────────
info "Step 3: Undeploying RHODS operator..."
bash "$DIR/04-rhods-operator/04-teardown.sh" || warn "04-teardown.sh failed"

# ── Step 4: Delete OTel collector, Tempo instance, and operators ────────────
info "Step 4: Deleting OTel collector..."
safe_delete -f "$DIR/03-tempo/03c-collector.yaml"

info "Step 4b: Deleting Tempo instance..."
safe_delete -f "$DIR/03-tempo/03b-instance.yaml"

info "Step 4c: Deleting Tempo operator..."
safe_delete -f "$DIR/03-tempo/03a-operator.yaml"

# ── Step 5: Delete Connectivity Link operator ───────────────────────────────
info "Step 5: Deleting Connectivity Link operator..."
safe_delete -f "$DIR/02-connectivity-link/02a-operator.yaml"

# ── Step 6: Delete OpenTelemetry operator ───────────────────────────────────
info "Step 6: Deleting OpenTelemetry operator..."
safe_delete -f "$DIR/01-otel/01a-operator.yaml"

# ── Step 7: Delete namespace ────────────────────────────────────────────────
info "Step 7: Deleting namespace..."
safe_delete -f "$DIR/00-namespace.yaml"

info "Teardown complete."
