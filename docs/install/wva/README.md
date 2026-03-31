# WVA (Workload Variant Autoscaler) Installation Guide

This guide walks through deploying the WVA stack on an OpenShift cluster with
ODH (Open Data Hub), including all dependencies and auth configuration needed
for KEDA to query Thanos Querier.

## Prerequisites

- An OpenShift 4.x cluster
- `oc` / `kubectl` CLI with cluster-admin access
- ODH operator installed (`redhat-ods-operator` namespace)

## Installation Order

Resources must be applied in order since later steps depend on earlier ones.

### Step 0 — Enable User Workload Monitoring

Enables the OpenShift user workload monitoring stack so that application metrics
(e.g. from vLLM / inference sim) are scraped and available in Thanos.

```bash
oc apply -f docs/install/wva/00-uwm-cm.yaml
```

### Step 1 — Install Custom Metrics Autoscaler (KEDA) Operator

Installs the Red Hat KEDA operator via OLM. This creates the `openshift-keda`
namespace, OperatorGroup, and Subscription.

```bash
oc apply -f docs/install/wva/01-custom-metrics-autoscaler.yaml
```

Wait for the operator pod to be ready:

```bash
oc get pods -n openshift-keda -w
```

### Step 2 — Install Red Hat Connectivity Link

Installs the Connectivity Link operator, which provides the Gateway API support
and the `AuthPolicy` CRD required by the llmisvc controller. The llmisvc
controller will refuse to create HTTPRoutes (to prevent unauthenticated
exposure) if this CRD is not present.

```bash
oc apply -f docs/install/wva/02-connectivity-link.yaml
```

Wait for the operator to be ready:

```bash
oc get csv -n openshift-operators | grep connectivity
```

### Step 3 — Create KedaController

Creates the `KedaController` CR which tells the KEDA operator to deploy the
actual KEDA components (keda-operator, keda-metrics-apiserver,
keda-admission). Without this, `ScaledObject` resources are ignored.

```bash
oc apply -f docs/install/wva/03-keda-controller.yaml
```

Wait for all KEDA pods to be running:

```bash
oc get pods -n openshift-keda -w
```

### Step 4 — Deploy and Patch ODH Operator

Deploys the ODH operator and patches the controller-manager ServiceAccount with
the pull secret needed to pull RHOAI images.

> **Source change required:** Before building, Gateway API must be enabled in
> the operator's bundled `inferenceservice-config`. The following changes have
> been made to
> `prefetched-manifests/kserve/overlays/odh/patches/inferenceservice-config-patch.yaml`:
>
> - **Line 29:** `"enableGatewayApi": true` (was `false`)
>
> **Why this is needed:** On OpenShift, the operator uses the `overlays/odh`
> kustomize overlay (selected at `kserve_controller_actions.go:37`), which
> hardcodes `enableGatewayApi: false`. The `overlays/odh-xks` overlay enables
> it, but is only used on vanilla Kubernetes clusters. There is currently no
> DSC, DSCI, or GatewayConfig field that dynamically toggles this — the
> `updateInferenceCM` function in `kserve_support.go` only sets
> `disableIngressCreation` and `serviceClusterIPNone`, never `enableGatewayApi`.
> Without this change, the llmisvc controller will not create HTTPRoutes,
> InferencePools, or the scheduler pod. The runtime `inferenceservice-config`
> configmap cannot be patched either, as the ODH operator will revert it on
> its next reconciliation cycle.
>
> The `kserveIngressGateway` value was also changed from
> `openshift-ingress/openshift-ai-inference` to
> `openshift-ingress/data-science-gateway`. The original value is the gateway
> name used in production OperatorHub deployments, while `data-science-gateway`
> is the one created by the ODH operator when built from source (see
> `gateway_support.go:41`). Like `enableGatewayApi`, this value is hardcoded
> in the kustomize overlay and never set dynamically at runtime.

```bash
bash docs/install/wva/04-deploy-and-patch-odh-operator-with-sa.sh
```

### Step 5 — Create DataScienceCluster

Creates the DSC with KServe and WVA managed. This triggers the ODH operator to
deploy the kserve controller, llmisvc controller, and WVA controller.

> **Note:** The operator automatically creates a `DSCInitialization` (DSCI) CR
> with sensible defaults — you do not need to create one manually.

```bash
oc apply -f docs/install/wva/05-wva-dsc.yaml
```

Wait for the controllers to come up:

```bash
oc get pods -n opendatahub -w
```

> **Important ordering note:** The Connectivity Link operator (step 2) must be
> installed and its CRDs available **before** the llmisvc controller starts. If
> the llmisvc controller starts before the `AuthPolicy` CRD exists, it caches
> that the CRD is unavailable and will block `LLMInferenceService` resources
> with `GatewayPreconditionNotMet`. If this happens, restart the llmisvc
> controller pod:
>
> ```bash
> oc rollout restart deployment llmisvc-controller-manager -n opendatahub
> ```

### Step 7 — KEDA Auth for Thanos Querier

KEDA needs to authenticate to OpenShift's Thanos Querier to read Prometheus
metrics for scaling decisions. These resources set up bearer token auth:

- **07** — `ServiceAccount` (`keda-metrics-reader`) in `openshift-keda`
- **08** — `ClusterRoleBinding` granting `cluster-monitoring-view` to the SA
- **09** — Long-lived SA token `Secret` + `ClusterTriggerAuthentication`
  (`ai-inference-keda-thanos`) that KEDA uses to authenticate
- **10** — JSON patch for `inferenceservice-config` to configure the llmisvc
  controller to generate `ScaledObjects` with `authenticationRef`

Apply the YAML resources:

```bash
oc apply -f docs/install/wva/07-auth/07-service-account.yaml
oc apply -f docs/install/wva/07-auth/08-cluster-role-binding.yaml
oc apply -f docs/install/wva/07-auth/09-trigger-authentication.yaml
```

Then patch the `inferenceservice-config` configmap so the llmisvc controller
generates `ScaledObjects` with the correct Thanos URL (port 9091) and auth
reference:

```bash
kubectl patch configmap inferenceservice-config -n opendatahub \
  --type='json' \
  --patch-file=docs/install/wva/07-auth/10-inferenceservice-config-patch.json
```

Restart the llmisvc controller to pick up the config change:

```bash
oc rollout restart deployment llmisvc-controller-manager -n opendatahub
```

### Step 11 — Deploy an LLMInferenceService

Deploy the example sim-based LLMInferenceService:

```bash
oc apply -f docs/install/wva/11-sim_llmisvc.yaml
```

> **Note:** The sim container serves plain HTTP. The llmisvc controller injects
> HTTPS probes by default, so the LLMISVC spec includes explicit HTTP probes to
> override this.

## Verification

Check that everything is healthy:

```bash
# LLMInferenceService is Ready
oc get llmisvc -n greg

# ScaledObject is Ready and has authentication
oc get scaledobject -n greg

# Inference pod is running
oc get pods -n greg
```

The `ScaledObject` should show `READY=True` and list `ai-inference-keda-thanos` under
`AUTHENTICATIONS`. `ACTIVE=False` is normal when the `wva_desired_replicas`
metric is at or below the threshold.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `GatewayPreconditionNotMet` on LLMISVC | llmisvc controller started before Connectivity Link CRDs were available | Restart the llmisvc controller pod |
| ScaledObject not Ready, KEDA logs `401 Unauthorized` | Missing bearer token auth for Thanos | Apply the auth resources (step 7) |
| ScaledObject not Ready, KEDA logs `400 Bad Request` | Wrong Thanos port (9092 is tenant-scoped) | Use port 9091 in the inferenceservice-config patch |
| Startup probe failing with "HTTP response to HTTPS client" | Inference container serves HTTP but probe uses HTTPS | Add explicit HTTP probes in the LLMISVC spec |
| `ScaledObject` missing `authenticationRef` after patching | llmisvc controller overwrites the ScaledObject | Patch the `inferenceservice-config` configmap instead, then restart the controller |
