#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> Installing Red Hat build of OpenTelemetry operator..."
kubectl apply -f "$DIR/01a-operator.yaml"

echo "==> Waiting for OpenTelemetryCollector CRD to be available..."
timeout=300; elapsed=0
while ! kubectl get crd opentelemetrycollectors.opentelemetry.io &>/dev/null; do
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: timed out waiting for OpenTelemetryCollector CRD"
        exit 1
    fi
    sleep 10
    elapsed=$((elapsed + 10))
done
echo "==> OpenTelemetry operator is ready."
