# xks-gateway

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

XKS Gateway onboarding chart - installs the GatewayConfig CRD, gateway namespace, and GatewayConfig CR for non-OpenShift Kubernetes.

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Red Hat |  |  |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| cleanupOnDisable | bool | `true` | Delete the release-owned GatewayConfig on upgrade when the gateway is disabled or unconfigured. |
| enabled | bool | `true` | Enable or disable the xks-gateway subchart. When used as a dependency of rhai-on-xks-chart, this controls whether the GatewayConfig CRD is installed or updated, and whether the gateway namespace, GatewayConfig CR, and OIDC resources are created. Helm retains an installed CRD as a cluster resource when this chart is disabled or uninstalled. |
| gateway.certificate.secretName | string | `""` | Name of TLS Secret (required when type is Provided) |
| gateway.certificate.type | string | `"SelfSigned"` | TLS strategy: SelfSigned (auto-generated) or Provided (BYO secret in gateway namespace) |
| gateway.cookie.expire | string | `"24h"` | Session cookie expiry duration (e.g., "24h", "8h") |
| gateway.cookie.refresh | string | `"1h"` | Access token refresh interval — must be less than the OIDC provider's Access Token Lifespan |
| gateway.domain | required | `""` | Base DNS domain (e.g., example.com). The operator prefixes gateway.subdomain (default rh-ai); wildcards are not supported. |
| gateway.ingressMode | string | `"LoadBalancer"` | How the gateway is exposed externally on XKS (LoadBalancer only) |
| gateway.namespace | string | `"rh-ai-gateway"` | Namespace created by this chart. Must be rh-ai-gateway; the operator hardcodes that name. |
| gateway.networkPolicy.ingress.enabled | bool | `true` | Enable ingress NetworkPolicy for kube-auth-proxy |
| gateway.oidc.clientID | required | `""` | OIDC client ID |
| gateway.oidc.clientSecretRef | object | `{"key":"client-secret","name":""}` | Reference to the OIDC client secret (name required for BYO mode; optional when oidcClientSecret is set, defaults to oidc-client-secret) |
| gateway.oidc.clientSecretRef.key | string | `"client-secret"` | Key within the Secret that holds the client secret value |
| gateway.oidc.clientSecretRef.name | string | `""` | Name of the Kubernetes Secret (existing or chart-created) |
| gateway.oidc.issuerURL | required | `""` | OIDC provider URL (e.g., https://keycloak.example.com/realms/rhai) |
| gateway.oidc.oidcClientSecret | string | `""` | OIDC client secret value; when set, the chart creates the Secret in gateway.namespace. Prefer referencing an existing Secret via clientSecretRef (BYO) for production; use --set-file with oidcClientSecret only for dev/test (the value is stored in the Helm release Secret). |
| gateway.oidc.secretNamespace | string | `""` | Namespace where the client secret is located (defaults to gateway.namespace if empty) |
| gateway.providerCASecretName | string | `""` | Name of Secret containing CA cert for the auth provider (must have ca.crt key, in gateway namespace) |
| gateway.subdomain | string | `""` | Subdomain prefix for the gateway |
| gateway.verifyProviderCertificate | bool | `true` | Verify auth provider TLS certificate (set to false only for development) |
| hooks.cliImage | string | `"registry.redhat.io/openshift4/ose-cli-rhel9:v4.20@sha256:d876c1d98b39d65c00c4261431bb84b90284699f3aef84d8701a25c786fb79a1"` | Image containing kubectl and bash for lifecycle hooks. |
| hooks.imagePullSecretName | string | `""` | Optional pull Secret for the GatewayConfig lifecycle Jobs. |
| hooks.resources | object | `{"limits":{"cpu":"200m","memory":"512Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}` | Resources for the lifecycle hook Job. |

