#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> Installing Red Hat Connectivity Link operator..."
kubectl apply -f "$DIR/02a-operator.yaml"
