# xks-gateway

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

XKS Gateway onboarding chart - installs the GatewayConfig CRD, gateway namespace, and GatewayConfig CR for non-OpenShift Kubernetes.

## Usage

This chart is a **subchart** of `rhai-on-xks-chart` (disabled by default in the parent chart). Configure it via `xks-gateway.gateway.*` values when installing the parent chart:

```bash
helm upgrade --install rhai-on-xks ./charts/rhai-on-xks-chart \
  --set xks-gateway.enabled=true \
  --set xks-gateway.gateway.domain=example.com \
  --set xks-gateway.gateway.oidc.issuerURL=https://keycloak.example.com/realms/rhai \
  --set xks-gateway.gateway.oidc.clientID=rhai-client \
  --set xks-gateway.gateway.oidc.clientSecretRef.name=my-oidc-secret
```

When the chart is enabled and `gateway.domain` is empty, only the GatewayConfig CRD is installed and no gateway resources are created. It can also be installed standalone for testing.

An existing `GatewayConfig` does not supply Helm values during an upgrade. Helm reuses saved release
values if no new values are supplied. For a partial upgrade, pass `--reuse-values` with the new
values, or supply a complete values file with the gateway settings. Setting only
`xks-gateway.enabled=true` in a parent-chart upgrade leaves `gateway.domain` empty and deletes
`default-gateway` in the pre-upgrade hook when `cleanupOnDisable` is enabled (the default).

Set `gateway.domain` to the base DNS domain, such as `example.com`. The operator prefixes
`gateway.subdomain` (default `rh-ai`), producing `rh-ai.example.com` by default. Do not include
`*.` in `gateway.domain`; it would produce an invalid Gateway listener hostname. A wildcard
DNS record or TLS certificate can still cover the resulting hostname.

## CRD handling

The GatewayConfig CRD is a regular Helm template, rendered from `files/gatewayconfig-crd.yaml`
when the chart is enabled. Helm creates and updates the CRD with the release. The CRD has
`helm.sh/resource-policy: keep`, so Helm retains it as a cluster resource when the chart is disabled
or uninstalled. Helm no longer manages it while it is outside the release.
A post-install/post-upgrade hook waits for the CRD to become Established, then creates or updates
`default-gateway` after Helm has created the gateway namespace and chart-managed OIDC Secret.

- The subchart's pre-upgrade hook deletes the release-owned `default-gateway` when the gateway is
  disabled or unconfigured and `cleanupOnDisable` is true (default). Set
  `xks-gateway.cleanupOnDisable=false` to retain it. The pre-delete hook deletes it on uninstall.
- The retained CRD and any other GatewayConfig objects remain available cluster-wide. If the API is
  no longer needed, first confirm no GatewayConfig objects should be kept, then explicitly delete
  the CRD with `kubectl delete crd gatewayconfigs.services.platform.opendatahub.io`. Deleting the
  CRD removes every GatewayConfig object.
- GatewayConfig schema changes merged into `rhods-operator` are automatically synchronized into this chart by the [GitOps sync workflow](https://github.com/red-hat-data-services/rhods-operator/blob/main/.github/workflows/trigger-gitops-sync.yaml).
- For local or manual updates, regenerate the CRD in the operator repository, then run `./scripts/sync-gatewayconfig-crd.sh /path/to/operator-repository` from this chart directory.

The upstream CRD currently permits wildcard `spec.domain` values. This chart rejects them because
the operator prefixes a subdomain when creating a Gateway listener on XKS.

The same lifecycle hooks are used when this chart is installed standalone or as a dependency of
`rhai-on-xks-chart`.

## OIDC client secret

**Recommended (production):** create a Kubernetes Secret in `rh-ai-gateway` and set `gateway.oidc.clientSecretRef.name` (and `key` if not `client-secret`).

**Dev/test only:** set `gateway.oidc.oidcClientSecret` (or `--set-file`) to have the chart create the Secret. The value is stored in the Helm release Secret.

## Namespace

When the gateway is configured, the chart creates `rh-ai-gateway`. The operator hardcodes this namespace; `gateway.namespace` cannot be changed.

The namespace has `helm.sh/resource-policy: keep` so `helm uninstall` does not delete workloads in `rh-ai-gateway`.

## Values

See the generated [API docs](api-docs.md#values) for the complete, current values reference.
