# LLM InferenceService Dashboards — Use Cases & User Queries

This document maps real operational questions to the dashboard panels and PromQL queries that answer them. Use it as a guide to understand what the monitoring suite covers and where to look when something goes wrong.

**Dashboard abbreviations** used in panel references below:

| Abbreviation | Dashboard File | UID |
|---|---|---|
| **CH** | `model-serving-llms-cluster-health-odc.json` | `model-serving-llms-cluster-health` |
| **MP** | `model-serving-llms-model-performance-usage-odc.json` | `model-serving-llms-model-performance` |
| **RD** | `model-serving-llms-replica-detail-odc.json` | `model-serving-llms-replica-detail` |
| **FD** | `model-serving-llms-failure-diagnostics-odc.json` | `model-serving-llms-failure-diagnostics` |

## Personas

| Persona                              | Role                             | Primary Concern                                       |
|--------------------------------------|----------------------------------|-------------------------------------------------------|
| **Platform Operator / SRE**          | Owns the serving infrastructure  | Is the cluster healthy? What broke?                   |
| **Data Scientist / Model Owner**     | Owns a specific model deployment | Is my model performing well? Where is the bottleneck? |
| **Infrastructure Engineer / DevOps** | Owns pod-level operations        | Which replica is misbehaving? Why?                    |

## Use Cases

### 1. Cluster-Wide Health Check

> "I just started my shift. Is everything running fine?"

**Dashboard**: Cluster Health Overview

Start at the top with **Gateway & Ingress** — these panels show traffic arriving at the cluster before it reaches any model:

- **Gateway Request Rate by Gateway** — is traffic flowing through the ingress layer?
- **Gateway Request Rate by Response Code** — full traffic profile by HTTP status code (200s, 4xx, 5xx)
- **Gateway Latency P95** — how much time is spent at the ingress layer?

If the gateway looks healthy, check the SLI summary gauges below. Four numbers tell you the state of the serving layer:

- **Total request rate** — is traffic flowing through to models?
- **HTTP error rate %** — are requests failing? (green < 1%, yellow >= 1%, red >= 5%)
- **E2E latency P99** — worst-case user-facing latency
- **Ready pods** — do we have enough capacity?

If all are green, the cluster is healthy. Move on.

#### Panels & Queries

**Gateway & Ingress:**

| Panel | ID | Query |
|---|---|---|
| CH: Gateway Request Rate by Gateway | 18 | `sum by (source_workload) (rate(istio_requests_total{llm_isvc_gateway="true",destination_service_namespace=~"$namespace"}[5m]))` |
| CH: Gateway Request Rate by Response Code | 19 | `sum by (response_code) (rate(istio_requests_total{llm_isvc_gateway="true",destination_service_namespace=~"$namespace"}[5m]))` |
| CH: Gateway Latency P95 by Gateway | 20 | `histogram_quantile(0.95, sum by (le, source_workload) (rate(istio_request_duration_milliseconds_bucket{llm_isvc_gateway="true",destination_service_namespace=~"$namespace"}[5m])))` |

**SLI Summary (resource inventory):**

| Panel | ID | Query |
|---|---|---|
| CH: LLMInferenceServices | 1 | `cluster:usage:resources:sum{resource="llminferenceservices.serving.kserve.io"} or vector(0)` |
| CH: LLMInferenceServiceConfigs | 2 | `cluster:usage:resources:sum{resource="llminferenceserviceconfigs.serving.kserve.io"} or vector(0)` |
| CH: Ready Pods | 4 | `sum(inference_pool_ready_pods{namespace=~"$namespace"}) or vector(0)` |

**SLI Summary (serving health):**

| Panel | ID | Query |
|---|---|---|
| CH: Total Request Rate | 3 | `sum(rate(vllm:request_success_total{namespace=~"$namespace"}[5m])) or vector(0)` |
| CH: HTTP Error Rate | 23 | `100 * (sum(rate(http_requests_total{llm_isvc_name!="",namespace=~"$namespace",status!="2xx"}[5m])) / (sum(rate(http_requests_total{llm_isvc_name!="",namespace=~"$namespace"}[5m])) > 0)) or vector(0)` |
| CH: E2E Latency P99 | 24 | `(histogram_quantile(0.99, sum(rate(vllm:e2e_request_latency_seconds_bucket{namespace=~"$namespace"}[5m])) by (le)) >= 0) or vector(0)` |

---

### 2. Identifying a Problematic Model Server

> "Something is off. Which model server is the problem?"

**Dashboard**: Cluster Health Overview → Per-Model-Server Health row

The per-model-server health panels show request rate, error rate, and latency by `llm_isvc_name` (the LLMInferenceService name). The problematic model server will stand out as a spike in error rate or latency. Click the model server name to drill down into Model Performance or Failure & Diagnostics.

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| CH: Request Rate by Model Server | 5 | `topk(10, sum(rate(vllm:request_success_total{namespace=~"$namespace"}[5m])) by (llm_isvc_name))` |
| CH: Error Rate by Model Server | 6 | `100 * (sum(rate(http_requests_total{llm_isvc_name!="",namespace=~"$namespace",status!="2xx"}[5m])) by (llm_isvc_name) / (sum(rate(http_requests_total{llm_isvc_name!="",namespace=~"$namespace"}[5m])) by (llm_isvc_name) > 0))` |
| CH: E2E Latency P95 by Model Server | 7 | `histogram_quantile(0.95, sum(rate(vllm:e2e_request_latency_seconds_bucket{namespace=~"$namespace"}[5m])) by (le, llm_isvc_name))` |

Panel 5 links to Model Performance, panel 6 links to Failure & Diagnostics (both pre-filtered to the selected model server).

---

### 3. Understanding Model-Level Latency

> "Users are complaining about slow responses from model X. Where is the time going?"

**Dashboard**: Model Performance & Usage

Three latency panels sit side by side:

- **E2E Request Latency** (P50/P90/P95/P99/Average) — total time from request to response
- **Time to First Token (TTFT)** — how long until the user sees the first token
- **Time per Output Token (TPOT)** — how fast tokens stream after the first one

If TTFT is high → the problem is in prefill (prompt processing). If TPOT is high → the problem is in decode (token generation). If both are normal but E2E is high → look at queue time.

The **Inter-Token Latency** panel (P50/P95/P99) shows token delivery smoothness — high values mean choppy streaming even if average TPOT looks fine.

The **Gateway Latency** row adds the client-facing perspective:

- **Gateway Latency per Model Server** (P50/P95/P99) — end-to-end latency as measured at the inference gateway (includes routing, scheduling, and inference)
- **Gateway vs Engine Latency (P99)** — overlays gateway P99 with vLLM engine P99. The delta reveals overhead from the gateway, Istio sidecar, and routing/scheduling layers

#### Panels & Queries

| Panel | ID | Query (representative percentile) |
|---|---|---|
| MP: End-to-End Request Latency | 6 | `histogram_quantile(0.95, sum(rate(vllm:e2e_request_latency_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| MP: Latency Trends (TTFT) | 7 | `histogram_quantile(0.95, sum(rate(vllm:time_to_first_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| MP: Time per Output Token (TPOT) | 8 | `histogram_quantile(0.95, sum(rate(vllm:time_per_output_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| MP: Inter-Token Latency | 160 | `histogram_quantile(0.99, sum(rate(vllm:inter_token_latency_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| MP: Gateway Latency per Model Server | 171 | `histogram_quantile(0.95, sum by (le, destination_canonical_service) (rate(istio_request_duration_milliseconds_bucket{llm_isvc_gateway="true",destination_service_namespace=~"$namespace",destination_canonical_service=~"$llm_isvc_name"}[5m])))` |
| MP: Gateway vs Engine Latency (P99) | 172 | Gateway: `histogram_quantile(0.99, sum(rate(istio_request_duration_milliseconds_bucket{llm_isvc_gateway="true",destination_service_namespace=~"$namespace",destination_canonical_service=~"$llm_isvc_name"}[5m])) by (le))` |
| | | Engine: `histogram_quantile(0.99, sum(rate(vllm:e2e_request_latency_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le)) * 1000` |

Each latency panel shows multiple percentiles (P50/P90/P95 or P50/P95/P99) as separate series. Panel 6 also includes an average computed as `sum(rate(..._sum[5m])) / sum(rate(..._count[5m]))`. Panel 172 converts the engine metric from seconds to milliseconds (`* 1000`) for direct comparison with the gateway metric.

---

### 4. Scheduling Bottleneck vs Compute Bottleneck

> "Latency is high — is it because we need more replicas, or because the engine itself is slow?"

**Dashboard**: Model Performance & Usage → Scheduling Delay vs Capacity row

The **Queue Time vs Inference Time P95** panel directly answers this:

- **Queue time >> Inference time** → not enough replicas or scheduling delay. Scale up pods.
- **Inference time high, Queue time low** → engine bottleneck (compute or KV cache). Check KV cache utilization and preemptions.
- **Both high** → the system is saturated end to end.

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| MP: Queue Time vs Inference Time (P95) | 131 | Queue: `histogram_quantile(0.95, sum(rate(vllm:request_queue_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| | | Inference: `histogram_quantile(0.95, sum(rate(vllm:request_inference_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| MP: Scheduler Queue Depth | 132 | `inference_pool_average_queue_size{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}` |
| | | `inference_pool_per_pod_queue_size{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}` |

The **Latency: Scheduler vs Engine** row provides a complementary view:

| Panel | ID | Query |
|---|---|---|
| MP: TTFT P99: Scheduler vs Engine | 141 | Scheduler: `histogram_quantile(0.99, sum(rate(llm_d_epp_request_ttft_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le, model_name))` |
| | | Engine: `histogram_quantile(0.99, sum(rate(vllm:time_to_first_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |
| MP: TPOT P99: Scheduler vs Engine | 142 | Scheduler: `histogram_quantile(0.99, sum(rate(inference_objective_normalized_time_per_output_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le, model_name))` |
| | | Engine: `histogram_quantile(0.99, sum(rate(vllm:time_per_output_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (le))` |

The delta between Scheduler (E2E) and Engine (vLLM) reveals scheduling/routing overhead.

---

### 5. Prefill vs Decode Phase Diagnosis

> "We run disaggregated Prefill/Decode. Which phase is the bottleneck?"

**Dashboard**: Model Performance & Usage → Prefill / Decode Phase Breakdown row

Use the `llm_isvc_role` variable to filter by phase. The row shows:

- **KV Cache by Phase** — is prefill or decode exhausting cache?
- **Requests Waiting by Phase** — which phase has the backlog?
- **TTFT by Phase** — time-to-first-token split by prefill vs decode
- **Prefill Time P95** and **Decode Time P95** — direct phase timing comparison

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| MP: KV Cache by Phase (Prefill vs Decode) | 101 | `avg(vllm:kv_cache_usage_perc{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}) by (llm_isvc_role) * 100` |
| MP: Requests Waiting by Phase | 102 | `sum(vllm:num_requests_waiting{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}) by (llm_isvc_role)` |
| MP: TTFT by Phase (P95) | 103 | `histogram_quantile(0.95, sum(rate(vllm:time_to_first_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (le, llm_isvc_role))` |
| MP: Prefill Time (P95) | 104 | `histogram_quantile(0.95, sum(rate(vllm:request_prefill_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (le, llm_isvc_role))` |
| MP: Decode Time (P95) | 105 | `histogram_quantile(0.95, sum(rate(vllm:request_decode_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (le, llm_isvc_role))` |

All queries in this row are broken down by `llm_isvc_role` (prefill/decode). These labels only appear on disaggregated deployments.

---

### 6. Wide EP / Multi-Node Topology Issues

> "We use multi-node inference with leader/worker topology. Are workers keeping up?"

**Dashboard**: Model Performance & Usage → Wide EP / Multi-Node Topology row

- **Request Volume by Component** — traffic distribution across leader/worker
- **KV Cache by Component** — which node type is running out of cache

An imbalance in request volume or a leader with saturated KV cache while workers are idle signals a routing or sharding problem.

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| MP: Request Volume by Component (Leader/Worker) | 111 | `sum(rate(vllm:request_success_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (llm_isvc_component)` |
| MP: KV Cache by Component (Leader/Worker) | 112 | `avg(vllm:kv_cache_usage_perc{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}) by (llm_isvc_component) * 100` |

The `llm_isvc_component` label distinguishes `workload-leader` from `workload-worker` in Wide EP deployments.

---

### 7. Identifying a Misbehaving Replica

> "Overall metrics look fine, but some users report intermittent slowness."

**Dashboard**: Replica Detail View

Every metric is broken down by pod. Look for outliers:

- One pod with higher **E2E latency** or **TTFT** than peers → possible hardware issue or uneven routing
- One pod with high **KV cache utilization** while others are low → unbalanced request distribution
- One pod with high **preemptions** → that pod is under memory pressure
- One pod with low **prefix cache hit rate** → cache cold-start or routing not prefix-aware

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| RD: E2E Latency P95 per Replica | 3 | `histogram_quantile(0.95, sum(rate(vllm:e2e_request_latency_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (le, pod))` |
| RD: TTFT P95 per Replica | 4 | `histogram_quantile(0.95, sum(rate(vllm:time_to_first_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (le, pod))` |
| RD: KV Cache Utilization per Replica | 8 | `avg(vllm:kv_cache_usage_perc{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}) by (pod) * 100` |
| RD: Preemptions per Replica | 104 | `sum(rate(vllm:num_preemptions_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (pod)` |
| RD: Prefix Cache Hit Rate per Replica | 103 | `100 * (sum(rate(vllm:prefix_cache_hits_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (pod) / (sum(rate(vllm:prefix_cache_queries_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (pod) > 0))` |

Additional per-replica panels available for deeper investigation:

| Panel | ID | Query |
|---|---|---|
| RD: Request Rate per Replica | 1 | `sum(rate(vllm:request_success_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (pod)` |
| RD: Error Rate per Replica | 2 | `100 * (rate(http_requests_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod",status!="2xx"}[5m]) / (rate(http_requests_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m]) > 0))` |
| RD: TPOT P95 per Replica | 5 | `histogram_quantile(0.95, sum(rate(vllm:time_per_output_token_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (le, pod))` |
| RD: Queue Time P95 per Replica | 11 | `histogram_quantile(0.95, sum(rate(vllm:request_queue_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (le, pod))` |
| RD: Requests Waiting per Replica | 6 | `sum(vllm:num_requests_waiting{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}) by (pod)` |
| RD: Requests Running per Replica | 7 | `sum(vllm:num_requests_running{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}) by (pod)` |
| RD: Avg Tokens per Iteration per Replica | 105 | `sum(rate(vllm:iteration_tokens_total_sum{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod"}[5m])) by (pod) / (sum(rate(vllm:iteration_tokens_total_count{...}[5m])) by (pod) > 0)` |

---

### 8. Understanding Error Types During an Incident

> "Error rate spiked. What kind of errors? What component is failing?"

**Dashboard**: Failure & Diagnostics

Start at the top with **Gateway & Auth Failures** to determine if the issue is at the ingress layer:

- **Gateway Errors by Model Server & Response Code** — are errors hitting specific model servers? What HTTP codes?
- **Gateway Response Flags** — transport-level issues: `DC` (downstream conn terminated), `NR` (no route), `DPE` (protocol error), `UH` (no healthy upstream)
- **Kuadrant Auth Decisions** — are requests being denied by the authorization gateway?
- **Gateway Latency by Response Code (P95)** — fast errors (e.g., 401 auth rejection) vs slow errors (e.g., 503 timeout after waiting)
- **Gateway Latency by Response Flags (P95)** — latency profile per Envoy failure mode

If the gateway looks clean, check the serving layer:

- **HTTP Request Rate (Success vs Error)** — magnitude of the failure
- **vLLM Request Outcomes** — are requests being aborted (engine failure), hitting max length (expected), or completing normally?
- **Scheduler Errors by Error Code** — specific error types at the scheduling layer

Then drill into functional area attribution:

- **Controller Reconcile Errors** → scheduling/routing layer is failing
- **Workqueue Health** → scheduler is falling behind (high depth) or retrying (high retry rate)
- **Memory Pressure** → preemptions + KV cache saturation

#### Panels & Queries

**Gateway & Auth Failures:**

| Panel | ID | Query |
|---|---|---|
| FD: Gateway Errors by Model Server & Response Code | 17 | `sum by (destination_canonical_service, response_code) (rate(istio_requests_total{llm_isvc_gateway="true",destination_service_namespace=~"$namespace",response_code!="200"}[5m]))` |
| FD: Gateway Response Flags | 18 | `sum by (response_flags) (rate(istio_requests_total{llm_isvc_gateway="true",destination_service_namespace=~"$namespace",response_flags!="-"}[5m]))` |
| FD: Kuadrant Auth Decisions | 19 | Allowed: `rate(kuadrant_allowed[5m])`, Denied: `rate(kuadrant_denied[5m])`, Errors: `rate(kuadrant_errors[5m])` |
| FD: Gateway Latency by Response Code (P95) | 20 | `histogram_quantile(0.95, sum by (le, response_code) (rate(istio_request_duration_milliseconds_bucket{llm_isvc_gateway="true",destination_service_namespace=~"$namespace",response_code!="200"}[5m])))` |
| FD: Gateway Latency by Response Flags (P95) | 21 | `histogram_quantile(0.95, sum by (le, response_flags) (rate(istio_request_duration_milliseconds_bucket{llm_isvc_gateway="true",destination_service_namespace=~"$namespace",response_flags!="-"}[5m])))` |

**Service-Level Failure Signals:**

| Panel | ID | Query |
|---|---|---|
| FD: HTTP Request Rate (Success vs Error) | 1 | Errors: `sum(rate(http_requests_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",status!="2xx"}[5m]))` |
| | | Success: `sum(rate(http_requests_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",status="2xx"}[5m]))` |
| FD: vLLM Request Outcomes | 2 | Aborted: `sum(rate(vllm:request_success_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",finished_reason="abort"}[5m]))` |
| | | Stop: `sum(rate(vllm:request_success_total{...,finished_reason="stop"}[5m]))` |
| | | Length: `sum(rate(vllm:request_success_total{...,finished_reason="length"}[5m]))` |
| FD: Scheduler Errors by Error Code | 12 | `sum by (error_code) (rate(inference_objective_request_error_total{namespace=~"$namespace"}[5m]))` |

**Functional Area Attribution:**

| Panel | ID | Query |
|---|---|---|
| FD: Scheduling Failures (Controller Errors) | 3 | `sum(rate(controller_runtime_reconcile_total{llm_isvc_name!="",namespace=~"$namespace",result="error"}[5m])) by (controller)` |
| FD: Scheduling Workqueue Health | 4 | Depth: `sum(workqueue_depth{llm_isvc_name!="",namespace=~"$namespace"}) by (controller)` |
| | | Retries: `sum(rate(workqueue_retries_total{llm_isvc_name!="",namespace=~"$namespace"}[5m])) by (controller)` |
| FD: Memory Pressure (Preemptions + KV Cache) | 5 | Preemptions: `sum(rate(vllm:num_preemptions_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m]))` |
| | | Max KV Cache: `max(vllm:kv_cache_usage_perc{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}) * 100` |

---

### 9. Failure Attribution by Phase and Topology

> "Errors are coming from vLLM aborts. Is it the prefill or decode phase? Leaders or workers?"

**Dashboard**: Failure & Diagnostics → Failures by Phase & Topology row

- **Abort Rate by Phase** — pinpoints whether aborts concentrate in prefill or decode
- **Abort Rate by Component** — pinpoints whether aborts concentrate on leaders or workers in Wide EP

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| FD: Abort Rate by Phase (Prefill vs Decode) | 6 | `sum(rate(vllm:request_success_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",finished_reason="abort"}[5m])) by (llm_isvc_role)` |
| FD: Abort Rate by Component (Leader/Worker) | 7 | `sum(rate(vllm:request_success_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",finished_reason="abort"}[5m])) by (llm_isvc_component)` |

---

### 10. Prefix Caching Effectiveness

> "We enabled prefix caching. Is it actually working?"

**Dashboard**: Model Performance & Usage → Caching Efficiency row

- **Prefix Cache Hit Rate** — percentage of queries that hit the prefix cache. Should be >0 and ideally >50% for workloads with shared system prompts.
- **Preemptions Rate** — high preemptions alongside low cache hit rate suggests the cache is being evicted under memory pressure.

For scheduler-side verification: Failure & Diagnostics → **Prefix Indexer Size** panel — if this isn't growing with traffic, prefix-aware routing may not be functioning.

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| MP: Prefix Cache Hit Rate | 121 | `100 * (sum(rate(vllm:prefix_cache_hits_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) / (sum(rate(vllm:prefix_cache_queries_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) > 0))` |
| MP: Preemptions Rate | 122 | `sum(rate(vllm:num_preemptions_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) by (llm_isvc_component)` |
| FD: Prefix Cache Hit Rate | 8 | `100 * (sum(rate(vllm:prefix_cache_hits_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) / (sum(rate(vllm:prefix_cache_queries_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) > 0))` |
| FD: Prefix Indexer Size | 9 | `inference_extension_prefix_indexer_size{namespace=~"$namespace"}` |

---

### 11. KV Offload Health in Disaggregated Deployments

> "We run Prefill/Decode disaggregation. Is the KV transfer between phases working?"

**Dashboard**: Replica Detail View → KV Offload & Inter-Token Latency row

- **KV Offload Throughput per Replica** — bytes/s of KV cache data being transferred. Zero means no transfers happening.
- **KV Offload Time per Replica** — time spent on transfers. High values indicate network or memory bandwidth constraints.
- **Inter-Token Latency P99 per Replica** — stalls in decode-phase token delivery may correlate with slow KV transfers.

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| RD: KV Offload Throughput per Replica | 201 | `rate(vllm:kv_offload_total_bytes_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod",llm_isvc_role=~"$llm_isvc_role",llm_isvc_component=~"$llm_isvc_component"}[5m])` |
| RD: KV Offload Time per Replica | 202 | `rate(vllm:kv_offload_total_time_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod",llm_isvc_role=~"$llm_isvc_role",llm_isvc_component=~"$llm_isvc_component"}[5m])` |
| RD: Inter-Token Latency P99 per Replica | 203 | `histogram_quantile(0.99, sum(rate(vllm:inter_token_latency_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",pod=~"$pod",llm_isvc_role=~"$llm_isvc_role",llm_isvc_component=~"$llm_isvc_component"}[5m])) by (le, pod))` |

---

### 12. EPP Scheduling Layer Health

> "The scheduler/EPP seems slow. Where is the overhead?"

**Dashboard**: Failure & Diagnostics → EPP Scheduling & Flow Control row

- **Scheduling Attempt Success Rate** — if failure rate is high, the scheduler can't find suitable endpoints
- **EPP Scheduling Latency P99** — total time from request arrival at EPP to endpoint selection
- **Plugin Processing Latency P99** — which scheduler plugin is slowest (filter, score, prefix-aware, etc.)
- **Flow Control Queue Duration P99** — if flow control is enabled, how long requests wait for admission

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| FD: Scheduling Attempt Success Rate | 13 | `sum by (status) (rate(inference_extension_scheduler_attempts_total{namespace=~"$namespace"}[5m]))` |
| FD: EPP Scheduling Latency P99 | 14 | `histogram_quantile(0.99, sum by (le) (rate(inference_extension_scheduler_e2e_duration_seconds_bucket{namespace=~"$namespace"}[5m])))` |
| FD: Plugin Processing Latency P99 | 15 | `histogram_quantile(0.99, sum by (le, plugin_type) (rate(inference_extension_plugin_duration_seconds_bucket{namespace=~"$namespace"}[5m])))` |
| FD: Flow Control Queue Duration P99 | 16 | `histogram_quantile(0.99, sum by (le) (rate(inference_extension_flow_control_request_queue_duration_seconds_bucket{namespace=~"$namespace"}[5m])))` |

---

### 13. Capacity Planning and Token Economics

> "How much token throughput is this cluster handling? Are we approaching limits?"

**Dashboard**: Cluster Health Overview → Token Throughput panel (cluster-wide input + output tokens/s)

**Dashboard**: Model Performance & Usage → Token Consumption panel (per-model breakdown of input vs output tokens)

Compare token throughput trends against KV cache utilization and queue depth to assess headroom.

#### Panels & Queries

**Cluster-wide throughput:**

| Panel | ID | Query |
|---|---|---|
| CH: Token Throughput (Cluster) | 12 | Input: `sum(rate(vllm:prompt_tokens_total{namespace=~"$namespace"}[5m]))` |
| | | Output: `sum(rate(vllm:generation_tokens_total{namespace=~"$namespace"}[5m]))` |

**Per-model breakdown:**

| Panel | ID | Query |
|---|---|---|
| MP: Token Consumption (Input vs Output) | 9 | Input: `sum(rate(vllm:prompt_tokens_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m]))` |
| | | Output: `sum(rate(vllm:generation_tokens_total{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m]))` |
| MP: Token Distribution per Request | 10 | Avg input: `sum(rate(vllm:request_prompt_tokens_sum{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace"}[5m])) / (sum(rate(vllm:request_prompt_tokens_count{...}[5m])) > 0)` |
| | | Avg output: `sum(rate(vllm:request_generation_tokens_sum{...}[5m])) / (sum(rate(vllm:request_generation_tokens_count{...}[5m])) > 0)` |

**Capacity signals to compare against:**

| Panel | ID | Query |
|---|---|---|
| CH: KV Cache Utilization by Model Server | 8 | `avg(vllm:kv_cache_usage_perc{namespace=~"$namespace"}) by (llm_isvc_name) * 100` |
| CH: Requests Waiting by Model Server | 9 | `sum(vllm:num_requests_waiting{namespace=~"$namespace"}) by (llm_isvc_name)` |

---

### 14. Data Freshness / Monitoring Pipeline Health

> "The dashboard shows no data. Is the monitoring pipeline broken?"

**Dashboard**: Cluster Health Overview → Data Staleness Detector

Shows seconds since last metric scrape per model server. Warning at 60s, critical at 300s. If staleness is high:

1. Check PodMonitor / ServiceMonitor scrape targets: `up{job=~".*kserve.*"}`
2. Verify LLMInferenceService is running and metrics collection is enabled
3. Check RBAC permissions for the ServiceAccount

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| CH: Data Staleness Detector | 11 | `time() - max(timestamp(vllm:num_requests_running{namespace=~"$namespace"})) by (llm_isvc_name)` |

---

### 15. SLO Violation Detection

> "Are we meeting our latency and availability SLOs?"

**Dashboard**: Cluster Health Overview → SLO & Scheduler Signals row

- **SLO Violations** — tracks `inference_objective_request_slo_violation_total` against configured objectives
- **Scheduler Error Rate by Model** — errors at the scheduling layer (distinct from vLLM-level errors)
- **Pool Saturation** — how close the pool is to capacity limits

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| CH: SLO Violations by Type | 13 | `sum by(model_name, type) (rate(inference_objective_request_slo_violation_total{namespace=~"$namespace"}[5m]))` |
| CH: Scheduler Error Rate by Model | 14 | `100 * (sum by(model_name) (rate(inference_objective_request_error_total{namespace=~"$namespace"}[5m])) / (sum by(model_name) (rate(inference_objective_request_total{namespace=~"$namespace"}[5m])) > 0))` |
| CH: Running Requests by Model (Scheduler) | 15 | `sum by(model_name) (inference_objective_running_requests{namespace=~"$namespace"})` |
| CH: Pool Saturation (Flow Control) | 16 | `inference_extension_flow_control_pool_saturation{namespace=~"$namespace"}` |
| CH: Scheduler Request Rate by Model | 17 | `sum by(model_name, target_model_name) (rate(inference_objective_request_total{namespace=~"$namespace"}[5m]))` |

The `model_name` label is the model name from the client request (the `model` field in the OpenAI API request). `target_model_name` is the model the request was actually routed to after InferenceModel rewrite rules.

---

### 16. NIXL KV Cache Transfer Health (Disaggregated Serving)

> "Is the KV cache transfer between prefill and decode instances working properly? How fast are transfers?"

**Dashboard**: Cluster Health Overview → Disaggregation Health row (cluster-wide failure signal)

Start with CH to see if any model server has NIXL transfer failures:

- **NIXL Failed Transfers by Model Server** — any non-zero rate means RDMA/network issues between prefill and decode instances
- **Engine Sleep State (Cluster-Wide)** — sleeping engines save GPU memory but cause cold-start latency on wakeup

If failures appear, drill into MP for the specific model:

**Dashboard**: Model Performance & Usage → NIXL Transfer Performance row

- **NIXL Transfer Latency P95 by Role** — is the transfer taking too long on prefill or decode side?
- **NIXL Post Time P95 by Role** — is the overhead in posting/initiating transfers?
- **NIXL Throughput (Bytes/s)** — total data volume flowing between instances
- **NIXL Descriptors per Transfer P95** — fragmented KV blocks increase transfer overhead

Then drill into RD for per-pod detail:

**Dashboard**: Replica Detail → NIXL Transfer Performance row

Per-replica variants of all NIXL metrics, plus KV Expired Requests and Engine Sleep State per pod.

#### Panels & Queries

**Cluster-wide (CH):**

| Panel | ID | Query |
|---|---|---|
| CH: NIXL Failed Transfers by Model Server | 601 | `sum by (llm_isvc_name) (rate(vllm:nixl_num_failed_transfers_total{llm_isvc_name!="",namespace=~"$namespace"}[5m]))` |
| CH: Engine Sleep State (Cluster-Wide) | 602 | `sum by (sleep_state) (vllm:engine_sleep_state{llm_isvc_name!="",namespace=~"$namespace"})` |

**Per-model (MP):**

| Panel | ID | Query |
|---|---|---|
| MP: NIXL Transfer Latency P95 by Role | 181 | `histogram_quantile(0.95, sum(rate(vllm:nixl_xfer_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (le, llm_isvc_role))` |
| MP: NIXL Post Time P95 by Role | 182 | `histogram_quantile(0.95, sum(rate(vllm:nixl_post_time_seconds_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (le, llm_isvc_role))` |
| MP: NIXL Throughput (Bytes/s) | 183 | `sum(rate(vllm:nixl_bytes_transferred_sum{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (llm_isvc_role)` |
| MP: NIXL Descriptors per Transfer P95 | 185 | `histogram_quantile(0.95, sum(rate(vllm:nixl_num_descriptors_bucket{llm_isvc_name=~"$llm_isvc_name",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (le, llm_isvc_role))` |

**Per-replica (RD):**

| Panel | ID | Query |
|---|---|---|
| RD: NIXL Transfer Time P95 per Replica | 301 | `histogram_quantile(0.95, sum(rate(vllm:nixl_xfer_time_seconds_bucket{...,pod=~"$pod"}[5m])) by (le, pod))` |
| RD: NIXL Post Time P95 per Replica | 302 | `histogram_quantile(0.95, sum(rate(vllm:nixl_post_time_seconds_bucket{...,pod=~"$pod"}[5m])) by (le, pod))` |
| RD: NIXL Bytes per Transfer P95 per Replica | 303 | `histogram_quantile(0.95, sum(rate(vllm:nixl_bytes_transferred_bucket{...,pod=~"$pod"}[5m])) by (le, pod))` |
| RD: NIXL Failed Transfers & Notifications per Replica | 304 | `rate(vllm:nixl_num_failed_transfers_total{...,pod=~"$pod"}[5m])` + `rate(vllm:nixl_num_failed_notifications_total{...}[5m])` |
| RD: NIXL KV Expired Requests per Replica | 305 | `rate(vllm:nixl_num_kv_expired_reqs_total{...,pod=~"$pod"}[5m])` |
| RD: Engine Sleep State per Replica | 306 | `vllm:engine_sleep_state{...,pod=~"$pod"}` |

---

### 17. Token Caching Efficiency (Cached vs Recomputed)

> "How effective is our prompt caching? Are we recomputing tokens we already cached?"

**Dashboard**: Model Performance & Usage → NIXL Transfer Performance row

- **Prompt Tokens: Cached vs Recomputed** — stacked view of cached tokens (local + external) vs tokens recomputed despite being cached. A high recomputed rate indicates prefix mismatches or cache invalidation pressure.

**Dashboard**: Failure & Diagnostics → NIXL & KV Transfer Failures row

- Same metric at aggregate level, useful for diagnosing caching layer issues alongside failure signals.

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| MP: Prompt Tokens: Cached vs Recomputed | 184 | Cached: `sum(rate(vllm:prompt_tokens_cached_total{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (llm_isvc_role)` |
| | | Recomputed: `sum(rate(vllm:prompt_tokens_recomputed_total{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m])) by (llm_isvc_role)` |
| FD: Prompt Tokens: Cached vs Recomputed | 704 | Cached: `sum(rate(vllm:prompt_tokens_cached_total{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m]))` |
| | | Recomputed: `sum(rate(vllm:prompt_tokens_recomputed_total{...}[5m]))` |

---

### 18. NIXL Failure Diagnostics

> "NIXL transfers are failing. Where exactly is the problem?"

**Dashboard**: Failure & Diagnostics → NIXL & KV Transfer Failures row

- **NIXL Failed Transfers & Notifications** — split by model server, shows both transfer failures (data path) and notification failures (control path)
- **NIXL KV Expired Requests** — requests whose KV expired before decode consumed it, indicating timing mismatch
- **Engine Sleep State Summary** — are engines sleeping and causing cold-start issues?

Then drill into RD for per-pod isolation: which specific pod is failing?

#### Panels & Queries

| Panel | ID | Query |
|---|---|---|
| FD: NIXL Failed Transfers & Notifications | 701 | Xfers: `sum by (llm_isvc_name) (rate(vllm:nixl_num_failed_transfers_total{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m]))` |
| | | Notifs: `sum by (llm_isvc_name) (rate(vllm:nixl_num_failed_notifications_total{...}[5m]))` |
| FD: NIXL KV Expired Requests | 702 | `sum by (llm_isvc_name) (rate(vllm:nixl_num_kv_expired_reqs_total{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"}[5m]))` |
| FD: Engine Sleep State Summary | 703 | `sum by (sleep_state) (vllm:engine_sleep_state{llm_isvc_name!="",namespace=~"$namespace",llm_isvc_role=~"$llm_isvc_role"})` |

---

## Drill-Down Workflow Summary

Most investigations follow this path:

```text
1. Cluster Health Overview          "Is something wrong?"
   └─ Gateway & Ingress row          (first check: is traffic reaching the cluster?)
   └─ SLI Summary gauges             (second check: is the serving layer healthy?)
   └─ Per-Model-Server Health row     (identify the problematic model server)
         |
         | (identify the model server)
         v
2a. Model Performance & Usage      "What kind of problem — latency, errors, capacity?"
         |
         | (need pod-level detail)
         v
3.  Replica Detail View             "Which specific pod? What's different about it?"

         — or —

2b. Failure & Diagnostics           "What type of failure? Which functional area?"
    └─ Gateway & Auth Failures row    (first check: is the error at the gateway?)
    └─ Service-Level Failures row     (second check: is the error in the serving layer?)
    └─ Functional Area Attribution    (which component is responsible?)
```

The dashboards pass namespace, model name, and time range between each other via navigation links. You never need to re-enter filters when drilling down.
