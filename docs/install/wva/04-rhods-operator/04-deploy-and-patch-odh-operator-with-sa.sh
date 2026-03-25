#!/bin/bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$DIR/../../../.."
IMG=quay.io/grpereir/odh-rhel9-operator:rhoai-3.4-ea.2-fixes

cd "$REPO_ROOT"

### Building local image for my debugging
# 1. Download upstream manifests into opt/manifests/
# 2. Overlay local prefetched-manifests/ changes on top (e.g. wva watch-namespace, enableGatewayApi)
# 3. Build and push the image
# RHOAI is the Makefile value (selects rhoai.Dockerfile, config/rhoai/ overlay).
# SelfManagedRHOAI is the runtime env var for the pod (see oc set env below).
ODH_PLATFORM_TYPE=RHOAI make get-manifests
cp -rf prefetched-manifests/* opt/manifests/
ODH_PLATFORM_TYPE=RHOAI make image-build image-push IMG=$IMG PLATFORM=linux/amd64 IMAGE_BUILD_FLAGS="--build-arg USE_LOCAL=true --build-arg CGO_ENABLED=1 --build-arg BUILDPLATFORM=linux/amd64 --build-arg TARGETPLATFORM=linux/amd64 --platform linux/amd64"
ODH_PLATFORM_TYPE=RHOAI make deploy IMG=$IMG

### Use the prebuilt image:
# IMG=quay.io/rhoai/odh-rhel9-operator:rhoai-3.4-ea.2 ODH_PLATFORM_TYPE=SelfManagedRHOAI make deploy

# The RHOAI manager.yaml comments out ODH_PLATFORM_TYPE, so the operator falls
# back to auto-detection which fails without a CatalogSource or rhods-operator CSV.
# Set it explicitly so the operator uses redhat-ods-applications as the app namespace.
oc set env deployment/rhods-operator -n redhat-ods-operator \
    ODH_PLATFORM_TYPE=SelfManagedRHOAI

kubectl create secret docker-registry rhoai-operator-pull-secret -n redhat-ods-operator \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json \
    --dry-run=client -o yaml | kubectl apply -f -

oc patch sa redhat-ods-operator-controller-manager -n redhat-ods-operator \
    -p '{"imagePullSecrets": [{"name": "rhoai-operator-pull-secret"}]}' --type=merge

kubectl delete pods -l "name=rhods-operator" -n redhat-ods-operator

# Pre-create pull secret in redhat-ods-applications for component controller images.
# The namespace is created by the operator on startup. The SA patches and pod restarts
# happen later in standup.sh after the DSC creates the controller deployments.
kubectl create secret docker-registry rhoai-operator-pull-secret -n redhat-ods-applications \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json \
    --dry-run=client -o yaml | kubectl apply -f -
