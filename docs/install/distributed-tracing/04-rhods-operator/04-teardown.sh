#!/bin/bash
# Undeploy the ODH operator via make undeploy.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$DIR/../../../.."

IMG=quay.io/rhoai/odh-rhel9-operator:rhoai-3.4  ODH_PLATFORM_TYPE=SelfManagedRHOAI make -C "$REPO_ROOT" undeploy
