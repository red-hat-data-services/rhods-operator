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
- cert-manager (for the webhook and metrics TargetAllocator mTLS)

## Installation

```bash
helm install odh-observability charts/odh-observability \
  --namespace odh-observability-system \
  --create-namespace \
  --set otelOperatorNamespace=<opentelemetry-operator-namespace>
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
| `otelOperatorNamespace` | string | `""` | Namespace of the active OpenTelemetry Operator service account used for TargetAllocator mTLS. Empty omits the supplemental cert-manager RBAC; the parent operator discovers and sets it. |
| `monitoringNamespace` | string | `""` | Namespace where Monitoring operands (Tempo, collector) are deployed, and the only namespace allowed to scrape operator metrics. Defaults to `operatorNamespace` when empty. On RHOAI this is DSCI `spec.monitoring.namespace`. |
| `image.repository` | string | `"quay.io/opendatahub/odh-observability"` | Controller container image repository |
| `image.tag` | string | `"odh-stable"` | Controller container image tag |
| `image.pullPolicy` | string | `"Always"` | Image pull policy |
| `gatewayNamespace` | string | `""` | Namespace containing the external router or gateway; empty uses the OpenShift ingress policy-group selector |
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
| `webhook.failurePolicy` | string | `"Fail"` | Webhook failure policy: `Fail` (fail closed) or `Ignore` (fail open). |
| `webhook.probe.namespace` | string | `""` | Namespace for the webhook admission probe canary. Defaults to `<fullname>-webhook-probe`. |
| `webhook.probe.createNamespace` | bool | `true` | Create the probe namespace labeled `monitoring.opendatahub.io/scrape=true`. When `false`, the namespace must already exist with that label. |
| `certManager.enabled` | bool | `true` | Enable cert-manager for webhook TLS provisioning |
| `certManager.certificateName` | string | `"odh-observability-webhook-cert"` | cert-manager Certificate resource name |
| `certManager.secretName` | string | `"odh-observability-webhook-cert"` | TLS secret name for webhook certs |
| `env` | list | `[]` | Extra environment variables (e.g. `RELATED_IMAGE_*` overrides) |
| `webhookIngressCIDRs` | list | `172.30.0.0/16`, `10.128.0.0/14`, `10.0.0.0/16` | CIDRs allowed to reach the admission webhook on port 9443. Defaults are the OpenShift and ROSA install defaults. See [Network policy](#network-policy). |
| `metrics.prometheusServiceAccount` | string | `data-science-monitoringstack-prometheus` | Prometheus ServiceAccount granted `get` on `/metrics`. The MonitoringStack creates it as `<monitoringstack-name>-prometheus` in `monitoringNamespace`. |
| `metrics.serviceMonitor.enabled` | bool | `false` | Create a ServiceMonitor for operator metrics. Enable only after the `monitoring.rhobs` ServiceMonitor CRD is installed. |
| `metrics.serviceMonitor.apiVersion` | string | `monitoring.rhobs/v1` | API version of that ServiceMonitor. |

## Network policy

The chart installs an ingress NetworkPolicy for the operator pod. Traffic that does not match a rule is dropped. A wrong value leaves the pod Ready.

| Port | Allowed from | What a wrong value looks like |
|------|--------------|-------------------------------|
| 8443 | `monitoringNamespace`, or `operatorNamespace` when that is empty | Operator metrics targets stay down. |
| 8081 | any source | Liveness and readiness fail if this port is restricted. Kubelet probes come from the node network and cannot be limited to a peer. |
| 9443 | `webhookIngressCIDRs` | Admission fails with `failed calling webhook` or `context deadline exceeded`. |

### `webhookIngressCIDRs` on ROSA

| Entry | ROSA default | Why it is in the list |
|-------|--------------|------------------------|
| `172.30.0.0/16` | service CIDR | service-network sources |
| `10.128.0.0/14` | pod CIDR | pod-network sources |
| `10.0.0.0/16` | machine CIDR | node IPs, which is where kube-apiserver traffic comes from |

kube-apiserver uses host networking, so the webhook packet source is a node IP inside the machine CIDR. On ROSA classic that node is a control-plane node. On ROSA with hosted control planes the control plane is outside the cluster; the call is proxied by the konnectivity agent, which also uses host networking, so the source is a worker node IP in the same machine CIDR.

There is no separate ROSA value. The machine CIDR is the entry that differs per cluster. It is fixed at install time and stays `10.0.0.0/16` unless the cluster was created with `--machine-cidr` because the VPC is different. The same node-IP check applies on ARO and self-managed OpenShift.

Read the ranges from the cluster being installed:

```bash
oc get network.config.openshift.io cluster \
  -o jsonpath='pod={.status.clusterNetwork[*].cidr}{"\n"}service={.status.serviceNetwork[*]}{"\n"}'

oc get nodes -o jsonpath='{range .items[*]}{.status.addresses[?(@.type=="InternalIP")].address}{"\n"}{end}'
```

Set `webhookIngressCIDRs` to the pod CIDR, the service CIDR, and a CIDR that contains those node InternalIPs.

```yaml
webhookIngressCIDRs:
  - 172.30.0.0/16
  - 10.128.0.0/14
  - 10.20.0.0/16   # this cluster's machine CIDR
```

### Metrics scrape settings

`metrics.serviceMonitor.enabled` stays `false` until a deployment turns it on, and only after the `monitoring.rhobs` ServiceMonitor CRD exists. `metrics.serviceMonitor.apiVersion` selects that CRD.

`metrics.prometheusServiceAccount` must be the Prometheus ServiceAccount in `monitoringNamespace`. The default matches the data-science MonitoringStack account (`<monitoringstack-name>-prometheus`). A mismatch returns 403 on scrape. The NetworkPolicy still allows the connection, so the pod and the policy both look healthy.

`monitoringNamespace` has to be that Prometheus namespace. The ServiceMonitor, the metrics reader binding, and the port 8443 rule all use it. An empty value falls back to `operatorNamespace`. On RHOAI, Prometheus runs in the DSCI monitoring namespace, so leaving this empty denies those scrapes.

## Further Reading

- [Architecture](../../docs/ARCHITECTURE.md)
- [Example Monitoring CRs](../../config/samples/)
