# odh-observability Helm Chart

This chart installs the odh-observability operator, which manages the full observability stack for Open Data Hub and Red Hat OpenShift AI.

The chart deploys:
- Operator Deployment
- ServiceAccount
- ClusterRole and ClusterRoleBinding
- Monitoring CRD (`services.platform.opendatahub.io/v1alpha1`)

## Prerequisites

- OpenShift 4.14+ or Kubernetes 1.27+
- Helm 3
- cert-manager (if `webhook.enabled` is true)

## Installation

```bash
helm install odh-observability charts/odh-observability \
  --namespace odh-observability-system \
  --create-namespace
```

After installation, create a [Monitoring CR](../../config/samples/) to configure the observability stack.

## Uninstall

```bash
helm uninstall odh-observability --namespace odh-observability-system
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `nameOverride` | string | `""` | Override the chart name |
| `fullnameOverride` | string | `""` | Override the fully qualified app name |
| `operatorNamespace` | string | `"opendatahub-operator-system"` | Namespace for all operator resources |
| `monitoringNamespace` | string | `""` | Namespace where Monitoring operands (Tempo, collector) are deployed. Defaults to `operatorNamespace` when empty. On RHOAI this is DSCI `spec.monitoring.namespace`. |
| `image.repository` | string | `"quay.io/opendatahub/odh-observability"` | Controller container image repository |
| `image.tag` | string | `"odh-stable"` | Controller container image tag |
| `image.pullPolicy` | string | `"Always"` | Image pull policy |
| `replicaCount` | int | `1` | Number of operator replicas |
| `resources.limits.cpu` | string | `"500m"` | CPU limit |
| `resources.limits.memory` | string | `"256Mi"` | Memory limit |
| `resources.requests.cpu` | string | `"100m"` | CPU request |
| `resources.requests.memory` | string | `"128Mi"` | Memory request |
| `leaderElect` | bool | `false` | Enable leader election (required when running multiple replicas) |
| `imagePullSecrets` | list | `[]` | Image pull secrets for private registries |
| `nodeSelector` | object | `{}` | Node selector for pod scheduling |
| `tolerations` | list | `[]` | Tolerations for pod scheduling |
| `affinity` | object | `{}` | Affinity rules for pod scheduling |
| `webhook.enabled` | bool | `true` | Enable the mutating admission webhook |
| `webhook.port` | int | `9443` | Webhook listener port |
| `certManager.enabled` | bool | `true` | Enable cert-manager for webhook TLS provisioning |
| `certManager.certificateName` | string | `"odh-observability-webhook-cert"` | cert-manager Certificate resource name |
| `certManager.secretName` | string | `"odh-observability-webhook-cert"` | TLS secret name for webhook certs |
| `env` | list | `[]` | Extra environment variables (e.g. `RELATED_IMAGE_*` overrides) |

## Further Reading

- [Architecture](../../docs/ARCHITECTURE.md)
- [Example Monitoring CRs](../../config/samples/)
