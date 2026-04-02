#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> Installing Red Hat build of Tempo operator..."
kubectl apply -f "$DIR/03a-operator.yaml"

echo "==> Waiting for TempoMonolithic CRD to be available..."
timeout=300; elapsed=0
while ! kubectl get crd tempomonolithics.tempo.grafana.com &>/dev/null; do
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: timed out waiting for TempoMonolithic CRD"
        exit 1
    fi
    sleep 10
    elapsed=$((elapsed + 10))
done

echo "==> Deploying TempoMonolithic instance..."
kubectl apply -f "$DIR/03b-instance.yaml"

echo "==> Waiting for Tempo to be ready..."
timeout=300; elapsed=0
while true; do
    ready=$(kubectl get tempomonolithic tracing -n distributed-tracing -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
    if [ "$ready" = "True" ]; then
        echo "==> Tempo is ready."
        break
    fi
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: timed out waiting for Tempo to be ready"
        exit 1
    fi
    sleep 10
    elapsed=$((elapsed + 10))
done

echo "==> Deploying OpenTelemetryCollector (must start after Tempo is ready)..."
kubectl apply -f "$DIR/03c-collector.yaml"
