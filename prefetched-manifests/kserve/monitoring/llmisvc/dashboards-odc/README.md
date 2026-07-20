# AI / Model Serving / LLMs - Monitoring Dashboards

This directory contains Grafana-compatible dashboards for monitoring LLMInferenceService deployments. The dashboards are
designed for the OpenShift Console monitoring plugin and are compatible with both the Administrator and Developer (ODC)
perspectives.

## Dashboard Architecture

The dashboards are organized as a drill-down hierarchy:

```text
Cluster Health Overview            "Is my cluster healthy?"
    |                       \
    v                        v
Model Performance & Usage    Failure & Diagnostics
    |                         "What kind of failure? Which component?"
    v
Replica Detail View           "Which specific pod is the problem?"
```

All dashboards include navigation links to move between levels without leaving the dashboard UI.

## Dashboard Overview

### 1. Cluster Health Overview (`model-serving-llms-cluster-health-odc.json`)

**Audience**: Platform Operators, SREs
**UID**: `model-serving-llms-cluster-health`
**Default Time Range**: 1 hour
**Perspective**: Administrator only

Single-pane-of-glass SLI-based health view. An operator opens this first and knows within seconds if the cluster is
healthy.

#### Key Metrics:

- **Gateway & Ingress**: Gateway request rate by gateway, requests by response code (full traffic profile), gateway
  latency P95 (Istio/Envoy metrics)
- **SLI Summary Gauges**: Total request rate, HTTP error rate %, E2E latency P99, ready pods
- **Per-Model-Server Health**: Request rate, error rate, and latency by model server (LLMInferenceService) with
  drill-down links
- **Capacity & Scheduling**: KV cache utilization, request queue depth, and ready pods by pool
- **Data Staleness Detector**: Seconds since last metric scrape per model server (warns at 60s, critical at 300s)
- **Token Throughput**: Cluster-wide input/output token processing rate
- **SLO & Scheduler Signals**: SLO violations by type, scheduler error rate, running requests, pool saturation,
  scheduler request rate

### 2. Model Performance & Usage (`model-serving-llms-model-performance-usage-odc.json`)

**Audience**: Data Scientists, Model Owners, MLOps Engineers
**UID**: `model-serving-llms-model-performance`
**Default Time Range**: 6 hours
**Perspective**: Administrator + Developer (ODC)

Detailed per-model performance with phase and topology awareness.

#### Key Metrics:

- **Error Rate**: HTTP-based error percentage with threshold alerts (1% warning, 5% critical)
- **Request Volume**: Total and per-component request throughput
- **Latency Analysis**: End-to-end, TTFT, and TPOT percentile tracking (P50, P90, P95)
- **Token Metrics**: Input vs output consumption rates, inter-token latency (P50/P95/P99), and per-request distribution
- **Phase Breakdown** (Prefill/Decode): KV cache, queue depth, TTFT, prefill time, decode time split by phase
- **Wide EP Topology**: Request volume and KV cache by component (leader/worker)
- **Caching Efficiency**: Prefix cache hit rate and preemption rate
- **Scheduling vs Capacity**: Queue time vs inference time comparison, scheduler queue depth
- **Gateway Latency**: Per-model gateway latency percentiles (P50/P95/P99) and gateway vs engine latency comparison -
  delta reveals gateway/routing/scheduling overhead
- **Scheduler vs Engine Latency**: TTFT and TPOT P99 comparison between scheduler (E2E, includes routing) and vLLM
  engine (compute only) - delta reveals scheduling overhead
- **Disaggregation & Token Sources**: Routing decisions, prompt token sources, prefix cache hit rate, token distribution

### 3. Replica Detail View (`model-serving-llms-replica-detail-odc.json`)

**Audience**: Infrastructure Engineers, DevOps
**UID**: `model-serving-llms-replica-detail`
**Default Time Range**: 6 hours
**Perspective**: Administrator + Developer (ODC)

Pod-level granularity for identifying hot spots and misbehaving replicas.

#### Key Metrics:

- **Per-Replica**: Request rate, error rate, E2E latency, TTFT, TPOT, queue time, KV cache, tokens
- **Phase Diagnostics per Replica**: Prefill time, decode time, prefix cache hit rate
- **Preemptions per Replica**: Identifies pods under KV cache memory pressure
- **Iteration Tokens per Replica**: Batch size consistency across pods
- **KV Offload & Inter-Token Latency**: KV offload throughput (bytes/s), KV offload time, inter-token latency P99 per
  replica

### 4. Failure & Diagnostics (`model-serving-llms-failure-diagnostics-odc.json`)

**Audience**: SREs, Platform Operators during incident response
**UID**: `model-serving-llms-failure-diagnostics`
**Default Time Range**: 6 hours
**Perspective**: Administrator + Developer (ODC)

Failure categorization by type and functional area attribution.

#### Key Metrics:

- **Gateway & Auth Failures**: Gateway errors by model server & response code, Envoy response flags (DC/NR/DPE/UH),
  Kuadrant auth decisions (allowed/denied/errors), gateway latency by response code and by response flags (P95) -
  distinguishes fast rejections from slow timeouts
- **Service-Level Failures**: HTTP success vs error rates, vLLM request outcomes (abort/stop/length)
- **Functional Area Attribution**: Controller reconcile errors (routing/scheduling), workqueue health, memory pressure
  signals
- **Failures by Phase & Topology**: Abort rate by phase (prefill/decode) and by component (leader/worker)
- **Caching Diagnostics**: Prefix cache hit rate, prefix indexer size
- **Scheduling vs Capacity**: Queue time vs inference time by phase, scheduler pool utilization
- **EPP Scheduling & Flow Control**: Scheduler errors by error code, scheduling attempt success rate, scheduling latency
  P99, plugin processing latency P99, flow control queue duration P99

## Metrics and Data Sources

### Available Metrics

The dashboards use Prometheus metrics from four sources:

- **PodMonitor** (`vllm:*`): vLLM engine metrics from port 8000
- **ServiceMonitor** (`inference_pool_*`, `inference_extension_*`, `inference_objective_*`): Scheduler (EPP) metrics
- **PodMonitor** (`istio_requests_total`, `istio_request_duration_milliseconds_bucket`): Gateway-level request and
  latency metrics from the inference gateway's Envoy sidecar, scraped via a per-gateway PodMonitor named
  `llm-inference-<gateway-name>`. The PodMonitor adds `llm_isvc_gateway="true"` via relabeling to disambiguate from
  unrelated gateways
- **Kuadrant** (`kuadrant_allowed`, `kuadrant_denied`, `kuadrant_errors`): Authorization gateway decision counters (
  cluster-wide, no per-model labels). Kuadrant auth panels require observability to be enabled
  (`Kuadrant.spec.observability.enable=true`), panels will show "No data" otherwise

### Metric Families

| Prefix                  | Source          | What it measures                                                                                               |
|-------------------------|-----------------|----------------------------------------------------------------------------------------------------------------|
| `vllm:*`                | vLLM engine     | Per-engine request rates, latency, KV cache, tokens                                                            |
| `inference_pool_*`      | Scheduler       | Pool-level aggregates (avg KV cache, avg queue, ready pods)                                                    |
| `inference_extension_*` | Scheduler       | Scheduling internals (plugin latency, prefix indexer, flow control)                                            |
| `inference_objective_*` | Scheduler       | Request-level metrics from the scheduler's perspective (E2E latency, TTFT, TPOT, SLO violations, error counts) |
| `istio_*`               | Envoy (Gateway) | Gateway-level request counts, latency histograms, payload sizes                                                |
| `kuadrant_*`            | Kuadrant        | Authorization decisions (allowed/denied/errors) - cluster-wide counters only                                   |

### Key Labels

- `llm_isvc_name`: LLMInferenceService identifier (added by KServe relabeling)
- `llm_isvc_role`: Phase role - `prefill` or `decode` (vLLM pods only, disaggregated deployments)
- `llm_isvc_component`: Component type - `workload`, `workload-prefill`, `workload-worker`, `workload-leader`,
  `router-scheduler`
- `model_name`: Model name from the client request (scheduler metrics only - this is the value of the `model` field in
  the OpenAI API request, which maps to an InferenceModel CRD)
- `target_model_name`: Model the request was actually routed to after InferenceModel rewrite rules
- `llm_isvc_gateway`: Sentinel label (`"true"`) added by the gateway PodMonitor relabeling - used to disambiguate LLM
  inference gateway metrics from unrelated gateways
- `gateway_name`: Gateway name extracted from the `gateway.networking.k8s.io/gateway-name` pod label (gateway PodMonitor
  relabeling)
- `source_workload`: Gateway workload identifier (Istio metrics only - identifies which gateway handled the request)
- `destination_canonical_service`: Destination service name (Istio metrics only - maps to LLMInferenceService name)
- `destination_service_namespace`: Destination namespace (Istio metrics only - used for namespace filtering)
- `response_code`: HTTP response code (Istio metrics only - e.g., 200, 401, 503)
- `response_flags`: Envoy response flags (Istio metrics only - e.g., `-` for OK, `DC`, `NR`, `DPE`, `UH`)
- `namespace`: Kubernetes namespace
- `pod`: Pod name

### Core Metric Examples

```promql
# Request rate
rate(vllm:request_success_total[5m])

# Error rate (HTTP-based)
100 * (sum(rate(http_requests_total{status!="2xx"}[5m])) / (sum(rate(http_requests_total[5m])) > 0))

# P95 latency
histogram_quantile(0.95, rate(vllm:time_to_first_token_seconds_bucket[5m]))

# Token throughput
rate(vllm:prompt_tokens_total[5m]) + rate(vllm:generation_tokens_total[5m])

# KV cache by phase
avg(vllm:kv_cache_usage_perc{llm_isvc_role="prefill"}) * 100

# Scheduling delay vs capacity
histogram_quantile(0.95, sum(rate(vllm:request_queue_time_seconds_bucket[5m])) by (le))
histogram_quantile(0.95, sum(rate(vllm:request_inference_time_seconds_bucket[5m])) by (le))

# Prefix cache hit rate
100 * sum(rate(vllm:prefix_cache_hits_total[5m])) / sum(rate(vllm:prefix_cache_queries_total[5m]))

# Staleness detection
time() - max(timestamp(vllm:num_requests_running)) by (llm_isvc_name)

# Gateway request rate by gateway (llm_isvc_gateway="true" disambiguates from unrelated gateways)
sum by (source_workload) (rate(istio_requests_total{llm_isvc_gateway="true",destination_service_namespace=~"$namespace"}[5m]))

# gateway request rate by response code
sum by (response_code) (rate(istio_requests_total{llm_isvc_gateway="true",destination_service_namespace=~"$namespace"}[5m]))

# Gateway latency P95
histogram_quantile(0.95, sum by (le, source_workload) (rate(istio_request_duration_milliseconds_bucket{llm_isvc_gateway="true",destination_service_namespace=~"$namespace"}[5m])))

# Kuadrant auth decisions
rate(kuadrant_allowed[5m])
rate(kuadrant_denied[5m])
```

## Dashboard Variables

### Common Variables

- **datasource**: Prometheus data source selection
- **namespace**: Filter by Kubernetes namespace (multi-select with "All" option)
- **llm_isvc_name**: Filter by LLMInferenceService name (multi-select with "All" option)

### Phase/Topology Variables (Model Performance, Replica Detail, Failure & Diagnostics)

- **llm_isvc_role**: Filter by phase role - Prefill/Decode (multi-select with "All" option)
- **llm_isvc_component**: Filter by component type (Replica Detail only)

### Time Ranges

Available refresh intervals: 10s, 30s, 1m, 5m, 15m, 30m, 1h, 2h, 1d

## Drill-Down Navigation

### From Cluster Health Overview

- Click any per-model-server series → **Model Performance & Usage** (pre-filtered to that model server)
- Click error rate series → **Failure & Diagnostics** (pre-filtered to that model server)
- Dashboard header links → All other dashboards

### Interpreting Scheduling vs Capacity

- **Queue time >> Inference time**: Problem is insufficient replicas or scheduling delay. Scale up pods.
- **Inference time high, Queue time low**: Problem is within the engine (compute, KV cache). Check KV cache utilization
  and preemptions.
- **High preemptions + High KV cache**: Memory pressure. Scale up or reduce concurrent requests.

## OCP Console Compatibility

These dashboards are rendered by the [openshift/monitoring-plugin](https://github.com/openshift/monitoring-plugin), not
native Grafana. The monitoring plugin loads Grafana-format JSON from ConfigMaps in `openshift-config-managed` with label
`console.openshift.io/dashboard=true`, but only supports a subset of Grafana's features.

### Supported Panel Types

The monitoring plugin supports exactly these panel types (see
`monitoring-plugin/web/src/components/dashboards/legacy/legacy-dashboard.tsx`):

| Panel Type               | Rendered As                                 | Used In Our Dashboards      |
|--------------------------|---------------------------------------------|-----------------------------|
| `graph`                  | Time-series line/area chart                 | All dashboards              |
| `gauge`                  | Single value display (same as `singlestat`) | Cluster Health (SLI gauges) |
| `singlestat`             | Single value display                        | -                           |
| `table`                  | Data table                                  | -                           |
| `grafana-piechart-panel` | Bar chart                                   | -                           |
| `row`                    | Collapsible section header                  | All dashboards              |

Panel types not in this list (e.g., `timeseries`, `stat`, `bargauge`) are **silently dropped** - they render as nothing.

### Layout System

The monitoring plugin uses a **12-column grid** via the `span` property on each panel, not Grafana's native 24-column
`gridPos` system.

The width resolution order is:

1. `panel.span` - integer 1-12 (preferred, what we use)
2. `panel.breakpoint` - percentage string (e.g., `"50%"`)
3. Default: `12` (full width)

**`gridPos.w` is ignored for layout.** We keep `gridPos` for Grafana import compatibility but rely on `span` for OCP
Console rendering. The mapping: `span = gridPos.w / 2`.

### Dashboard Format Requirements

- **Schema Version**: `schemaVersion: 22` (the plugin does not enforce a specific version but relies on field presence)
- **Datasource**: Simple string `"datasource": "$datasource"` - structured datasource objects are not supported
- **Panel layout**: Flat `panels` array with `type: "row"` panels as section markers. The plugin groups non-row panels
  under the preceding row panel automatically
- **Variables**: `templating.list` with `type: "query"` and `type: "interval"` are supported
- **NaN Protection**: Gauge panels use `or vector(0)` fallback to show "0" instead of "No data"; graph panels omit it to
  avoid phantom zero-value series with empty labels. Histogram quantiles in gauges use `(... >= 0) or vector(0)` to
  handle NaN from empty buckets
- **Generic Metric Disambiguation**: Metrics with common names (`http_requests_total`,
  `controller_runtime_reconcile_total`, `workqueue_depth`, `workqueue_retries_total`) must include `llm_isvc_name!=""`
  or `llm_isvc_name=~"$llm_isvc_name"` to scope to KServe-monitored pods. The `llm_isvc_name` label is added by the
  PodMonitor/ServiceMonitor relabeling rules in `monitoring.go` and is only present on metrics scraped from
  LLMInferenceService components. Gateway metrics (`istio_*`) use `llm_isvc_gateway="true"` instead - this sentinel
  label is added by the gateway PodMonitor (`llm-inference-<gateway-name>`) and ensures only metrics from LLM inference
  gateways are shown

### Access Control Labels

| Label                                        | Effect                                                                   |
|----------------------------------------------|--------------------------------------------------------------------------|
| `console.openshift.io/dashboard: "true"`     | Dashboard visible in Administrator perspective                           |
| `console.openshift.io/odc-dashboard: "true"` | Dashboard also visible in Developer (ODC) perspective (namespace-scoped) |

Cluster Health Overview is **Admin-only** because it queries cluster-wide metrics (controller reconcile errors,
cross-namespace aggregation). The other 3 dashboards are visible in both perspectives since EPP and vLLM run in user
namespaces.

## Deployment Topology Support

| Topology                          | Labels Available                            | Dashboard Coverage                                                               |
|-----------------------------------|---------------------------------------------|----------------------------------------------------------------------------------|
| **Single-node**                   | `llm_isvc_component=workload`               | All dashboards                                                                   |
| **Multi-node (Wide EP)**          | `llm_isvc_component=workload-leader/worker` | Wide EP rows in Model Performance, Component breakdown in Failure & Diagnostics  |
| **Prefill/Decode Disaggregation** | `llm_isvc_role=prefill/decode`              | Phase Breakdown rows in Model Performance, Phase panels in Failure & Diagnostics |

## Verifying Dashboard Deployment

Check if dashboards are deployed:

```bash
kubectl get configmap -n openshift-config-managed -l console.openshift.io/dashboard=true,app.kubernetes.io/part-of=kserve
```

Check monitoring pipeline is active:

```promql
count(up{job=~".*kserve-llm-isvc.*"}) > 0
```

## Installation

1. Import the JSON files into your Grafana instance or deploy via kustomize
2. Configure Prometheus data source with appropriate RBAC permissions
3. Ensure LLMInferenceService monitoring is enabled (see `pkg/controller/llmisvc/monitoring.go`)
4. Verify metric collection via PodMonitor and ServiceMonitor resources

## Known Metric Gaps

| Area                       | Gap                                                                       | Workaround                                                                          |
|----------------------------|---------------------------------------------------------------------------|-------------------------------------------------------------------------------------|
| GPU utilization            | Requires DCGM exporter                                                    | Out of scope - integrate DCGM separately                                            |
| HTTP status sub-categories | vLLM may only emit `status="2xx"`                                         | All non-2xx lumped as errors                                                        |
| KV transfer failures       | No metric for P/D transfer issues                                         | Monitor prefill/decode latency divergence                                           |
| Autoscaler events          | No scale-up/down decision metrics                                         | Track ready pod count over time                                                     |
| NCCL/inter-node failures   | Not instrumented in vLLM                                                  | Monitor Wide EP component abort rates                                               |
| Kuadrant per-model labels  | Kuadrant metrics are cluster-wide counters with no model/namespace labels | Use Istio `destination_canonical_service` for per-model gateway attribution instead |

## Troubleshooting

### Common Issues

1. **No data in panels**: Verify LLMInferenceService is running and metrics collection is enabled
2. **Permission errors**: Check ServiceAccount and ClusterRoleBinding for metrics access
3. **Missing labels**: Ensure relabeling configurations are applied in monitoring.go
4. **Phase filters empty**: `llm_isvc_role` only appears on disaggregated deployments
5. **Staleness panel shows high values**: Check PodMonitor/ServiceMonitor scrape targets with `up{job=~".*kserve.*"}`

### Debug Queries

```promql
# Check if metrics are being collected
{__name__=~"vllm:.*"}

# Verify label presence
vllm:request_success_total{llm_isvc_name!=""}

# Check monitoring components
up{job=~".*kserve.*"}

# Verify phase labels exist
vllm:request_success_total{llm_isvc_role!=""}
```
