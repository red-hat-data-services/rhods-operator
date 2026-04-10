#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$DIR/../../../.."

cd "$REPO_ROOT"

### Use the prebuilt image:
IMG=quay.io/rhoai/odh-rhel9-operator:rhoai-3.4 ODH_PLATFORM_TYPE=SelfManagedRHOAI make deploy # use OOTB manifests from 3.4 Stable

oc set env deployment/rhods-operator -n redhat-ods-operator \
    ODH_PLATFORM_TYPE=SelfManagedRHOAI

oc create secret docker-registry rhoai-operator-pull-secret -n redhat-ods-operator \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json \
    --dry-run=client -o yaml | oc apply -f -

oc patch sa redhat-ods-operator-controller-manager -n redhat-ods-operator \
    -p '{"imagePullSecrets": [{"name": "rhoai-operator-pull-secret"}]}' --type=merge

oc delete pods -l "name=rhods-operator" -n redhat-ods-operator

# Wait for the operator to create redhat-ods-applications namespace (via auto-DSCI),
# then pre-create the pull secret for component controller images.
echo "Waiting for redhat-ods-applications namespace..."
timeout=180; elapsed=0
while ! oc get namespace redhat-ods-applications &>/dev/null; do
    if [ "$elapsed" -ge "$timeout" ]; then
        echo "ERROR: timed out waiting for redhat-ods-applications namespace"
        exit 1
    fi
    sleep 5
    elapsed=$((elapsed + 5))
done
