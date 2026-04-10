# Enable Distributed Tracing for llm-d deployments

You can enable distributed tracing for your llm-d model deployments to gain visibility into the full request lifecycle — from the gateway, through the inference scheduler, and into the vLLM model server. Traces are collected using the Red Hat build of OpenTelemetry and stored in the Red Hat build of Tempo, with a Jaeger UI for visualization.

## Prerequisites

- You have an OpenShift cluster on version `4.20` or later.

- You have installed the OpenShift CLI (`oc`).

- You have logged in as a user with cluster-admin privileges.

- Compatible AI accelerators are available in the cluster.

- You have installed the `Red Hat Connectivity Link` operator from OperatorHub. For more information on how to do this, refer to the [Red Hat official documentation](https://docs.redhat.com/en/documentation/red_hat_connectivity_link/1.3/html/installing_on_openshift_container_platform/index) on installing it.

- You have the `Red Hat OpenShift Service Mesh 3` operator installed in your cluster. You should have this by default on any OpenShift cluster version `4.20` or later.

- You have installed {productname-long} {vernum}.

- A `DataScienceClusterInitialization` (DSCI) and `DataScienceCluster` (DSC) exist in your cluster, enabling the `llmisvc-controller-manager` and `kserve-controller-manager`. The `DataScienceClusterInitialization` gets created by the Red Hat OpenShift-AI operator out of the box for you. This is a sample excerpt from the `DataScienceCluster` manifest:

```yaml
apiVersion: datasciencecluster.opendatahub.io/v2
kind: DataScienceCluster
metadata:
  name: default-dsc
  labels:
    app.kubernetes.io/name: datasciencecluster
spec:
  components:
    kserve:
      managementState: "Managed"
      nim:
        managementState: "Managed"
      rawDeploymentServiceConfig: "Headed"
    ... # Other DSC components as desired
```

- No other `LLMInferenceService`s exist in the namespace you intend to deploy in (each namespace contains only one llm-d inference stack).

## Architecture Overview

The distributed tracing pipeline consists of three components:

1. **vLLM model server** — Instrumented with OpenTelemetry to emit trace spans for each inference request. The `--otlp-traces-endpoint` and `--collect-detailed-traces` flags enable this.
2. **OpenTelemetry Collector** — Receives traces from vLLM (and the inference scheduler), filters out noise (e.g., `/metrics` scraping spans), batches them, and forwards them to Tempo.
3. **Tempo** — Stores traces and exposes a Jaeger UI for querying and visualizing them.

```
vLLM (--otlp-traces-endpoint) ──► OTel Collector ──► Tempo ──► Jaeger UI
                                    ▲
Inference Scheduler (--tracing) ────┘
```

## Procedure

### Step 1 — Install the Red Hat build of OpenTelemetry operator

The OpenTelemetry operator provides the `OpenTelemetryCollector` CRD, which we use to deploy a managed collector instance. Install it from the `redhat-operators` catalog:

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-opentelemetry-operator
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: openshift-opentelemetry-operator
  namespace: openshift-opentelemetry-operator
spec:
  upgradeStrategy: Default
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: opentelemetry-product
  namespace: openshift-opentelemetry-operator
spec:
  channel: stable
  installPlanApproval: Automatic
  name: opentelemetry-product
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
```

Wait for the operator to be ready:

```bash
oc get csv -n openshift-opentelemetry-operator -w
```

You should see the `opentelemetry-operator` CSV reach `Succeeded`:

```console
NAME                              DISPLAY                           VERSION   REPLACES   PHASE
opentelemetry-operator.v0.x.x     Red Hat build of OpenTelemetry    0.x.x                Succeeded
```

### Step 2 — Install the Red Hat build of Tempo operator

The Tempo operator provides the `TempoMonolithic` CRD, which deploys a single-process Tempo instance suitable for development and demo environments. Install it from the `redhat-operators` catalog:

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-tempo-operator
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: openshift-tempo-operator
  namespace: openshift-tempo-operator
spec:
  upgradeStrategy: Default
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: tempo-product
  namespace: openshift-tempo-operator
spec:
  channel: stable
  installPlanApproval: Automatic
  name: tempo-product
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
```

Wait for the operator to be ready:

```bash
oc get csv -n openshift-tempo-operator -w
```

### Step 3 — Create the workload namespace

Because we only allow for one inference stack per namespace at this time, we will create the `distributed-tracing` namespace, although this should work for any namespace provided you adjust the manifests accordingly.

```bash
oc create ns distributed-tracing && oc project distributed-tracing
```

`oc` should respond by telling us our namespace was created and we are using that project:

```console
namespace/distributed-tracing created
Now using project "distributed-tracing" on server "https://api.example.com:443".
```

### Step 4 — Deploy the Tempo instance

Create a `TempoMonolithic` instance in the workload namespace. This stores traces in-memory and exposes a Jaeger UI via an OpenShift Route:

```bash
oc apply -f - <<'EOF'
apiVersion: tempo.grafana.com/v1alpha1
kind: TempoMonolithic
metadata:
  name: tracing
  namespace: distributed-tracing
spec:
  storage:
    traces:
      backend: memory
  jaegerui:
    enabled: true
    route:
      enabled: true
  ingestion:
    otlp:
      grpc:
        enabled: true
      http:
        enabled: true
EOF
```

Wait for Tempo to be ready:

```bash
oc get tempomonolithic tracing -n distributed-tracing -w
```

You should see the instance reach `Ready`:

```console
NAME      AGE
tracing   45s
```

Verify the Tempo pod is running:

```bash
oc get pods -n distributed-tracing -l app.kubernetes.io/instance=tempo-tracing
```

```console
NAME              READY   STATUS    RESTARTS   AGE
tempo-tracing-0   4/4     Running   0          60s
```

Some things to note about this Tempo setup:

1. **In-memory storage** — Traces are stored in memory and will be lost if the pod restarts. For production deployments, use `TempoStack` with object storage (e.g., S3 or ODF) instead.
2. **Jaeger UI** — The `jaegerui.enabled: true` and `route.enabled: true` settings deploy the Jaeger query UI and create an OpenShift Route. You can find the route with `oc get route -n distributed-tracing`.
3. **Multi-tenancy** — By default multi-tenancy is not enabled. OpenShift will emit a warning about this during creation. Enabling multi-tenancy (`multitenancy.enabled: true`, `mode: openshift`) adds auth to the ingest and query paths, but the Jaeger UI currently does not send the required `X-Scope-OrgID` tenant header, making the UI inaccessible. For demo purposes, leaving multi-tenancy disabled is recommended.

### Step 5 — Deploy the OpenTelemetry Collector

The collector must be deployed **after** Tempo is ready, otherwise its gRPC connection to Tempo will fail and traces will be silently dropped.

Create the `OpenTelemetryCollector` instance:

```bash
oc apply -f - <<'EOF'
apiVersion: opentelemetry.io/v1beta1
kind: OpenTelemetryCollector
metadata:
  name: otel
  namespace: distributed-tracing
spec:
  mode: deployment
  observability:
    metrics:
      enableMetrics: true
  config:
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: "0.0.0.0:4317"
          http:
            endpoint: "0.0.0.0:4318"
    processors:
      filter/drop-metrics-scraping:
        error_mode: ignore
        traces:
          span:
            - 'attributes["url.path"] == "/metrics"'
            - 'attributes["http.route"] == "/metrics"'
            - name == "GET /metrics"
            - name == "GET"
      batch:
        send_batch_size: 1024
        timeout: 1s
    exporters:
      otlp/tempo:
        endpoint: "tempo-tracing:4317"
        tls:
          insecure: true
    service:
      pipelines:
        traces:
          receivers: [otlp]
          processors: [filter/drop-metrics-scraping, batch]
          exporters: [otlp/tempo]
EOF
```

Verify the collector pod is running:

```bash
oc get pods -n distributed-tracing -l app.kubernetes.io/name=otel-collector
```

```console
NAME                              READY   STATUS    RESTARTS   AGE
otel-collector-5766fc4494-fdzlb   1/1     Running   0          30s
```

Some things to note about this collector configuration:

1. **Service name** — The operator creates a `Service` named `<name>-collector`, so naming the resource `otel` produces a service called `otel-collector` on ports `4317` (gRPC) and `4318` (HTTP). This is the endpoint that vLLM and the inference scheduler point their OTLP exporters at.
2. **Filter processor** — The `filter/drop-metrics-scraping` processor drops trace spans generated by Prometheus scraping the `/metrics` endpoint. These are noise and not useful for debugging inference requests.
3. **Batch processor** — Batches spans before export to reduce the number of gRPC calls to Tempo.
4. **Ordering** — If the collector is deployed before Tempo is ready, the gRPC client may cache a failed connection state and silently drop traces even after Tempo comes up. If you suspect this has happened, restart the collector pod: `oc delete pod -n distributed-tracing -l app.kubernetes.io/name=otel-collector`.

### Step 6 — Create the Gateway

Next we will create a gateway that uses TLS via OpenShift Service Mesh:

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: distributed-tracing-gateway-config
  namespace: distributed-tracing
data:
  service: |
    metadata:
      annotations:
        service.beta.openshift.io/serving-cert-secret-name: "distributed-tracing-gateway-tls"
    spec:
      type: ClusterIP
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: distributed-tracing-gateway
  namespace: distributed-tracing
spec:
  gatewayClassName: data-science-gateway-class
  infrastructure:
    parametersRef:
      group: ""
      kind: ConfigMap
      name: distributed-tracing-gateway-config
  listeners:
  - allowedRoutes:
      namespaces:
        from: Same
    name: https
    port: 443
    protocol: HTTPS
    tls:
      certificateRefs:
      - group: ""
        kind: Secret
        name: distributed-tracing-gateway-tls
      mode: Terminate
EOF
```

You should see:

```console
configmap/distributed-tracing-gateway-config created
gateway.gateway.networking.k8s.io/distributed-tracing-gateway created
```

Wait for the gateway to be programmed:

```bash
oc get gateway distributed-tracing-gateway -n distributed-tracing -w
```

```console
NAME                          CLASS                        ADDRESS   PROGRAMMED   AGE
distributed-tracing-gateway   data-science-gateway-class             True         30s
```

**Note:** We use `ClusterIP` as the service type because not all OpenShift flavours have `LoadBalancer` integration. For accessing the inference endpoint, you can use `kubectl port-forward` (as shown in the verification section) or send requests from a pod inside the cluster.

### Step 7 — Create the LLMInferenceService with tracing enabled

Now we create the `LLMInferenceService` with distributed tracing configured on both the inference scheduler and the vLLM model server:

```bash
oc apply -f - <<'EOF'
apiVersion: serving.kserve.io/v1alpha2
kind: LLMInferenceService
metadata:
  name: distributed-tracing-llama
  namespace: distributed-tracing
spec:
  router:
    scheduler:
      template:
        containers:
          - name: main
            args:
              - --tracing=true
            env:
              - name: OTEL_SERVICE_NAME
                value: "gateway-api-inference-extension"
              - name: OTEL_EXPORTER_OTLP_ENDPOINT
                value: "http://otel-collector:4317"
              - name: OTEL_TRACES_EXPORTER
                value: "otlp"
              - name: OTEL_RESOURCE_ATTRIBUTES_NODE_NAME
                valueFrom:
                  fieldRef:
                    apiVersion: v1
                    fieldPath: spec.nodeName
              - name: OTEL_RESOURCE_ATTRIBUTES_POD_NAME
                valueFrom:
                  fieldRef:
                    apiVersion: v1
                    fieldPath: metadata.name
              - name: OTEL_RESOURCE_ATTRIBUTES
                value: "k8s.namespace.name=$(NAMESPACE),k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME),k8s.pod.name=$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)"
              - name: OTEL_TRACES_SAMPLER
                value: "parentbased_traceidratio"
              - name: OTEL_TRACES_SAMPLER_ARG
                value: "0.1"
    route: {}
    gateway:
      refs:
      - name: distributed-tracing-gateway
        namespace: distributed-tracing
  model:
    uri: hf://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
  template:
    containers:
      - name: main
        image: registry.redhat.io/rhaii-early-access/vllm-cuda-rhel9:3.4.0-ea.2-1774939203
        imagePullPolicy: Always
        args:
          - --otlp-traces-endpoint
          - "http://otel-collector:4317"
          - --collect-detailed-traces
          - "all"
        env:
          - name: OTEL_SERVICE_NAME
            value: "vllm-decode"
          - name: OTEL_EXPORTER_OTLP_ENDPOINT
            value: "http://otel-collector:4317"
          - name: OTEL_TRACES_EXPORTER
            value: "otlp"
          - name: OTEL_TRACES_SAMPLER
            value: "parentbased_traceidratio"
          - name: OTEL_TRACES_SAMPLER_ARG
            value: "0.1"
        resources:
          limits:
            cpu: '4'
            memory: 32Gi
            nvidia.com/gpu: 1
          requests:
            cpu: '2'
            memory: 16Gi
            nvidia.com/gpu: 1
EOF
```

Wait for the LLMISVC to be ready:

```bash
oc get llminferenceservice distributed-tracing-llama -n distributed-tracing -w
```

Some important pieces to note about this configuration:

1. **Scheduler tracing** — The `--tracing=true` arg on the scheduler container enables trace propagation through the inference scheduler (the `gateway-api-inference-extension` component). The `OTEL_*` environment variables configure where and how traces are exported.

2. **vLLM tracing** — The `--otlp-traces-endpoint` and `--collect-detailed-traces all` args on the vLLM container enable the model server to emit detailed internal spans (tokenization, model execution, sampling, etc.) to the OTel Collector.

3. **Sampling rate** — Both the scheduler and vLLM are configured with `OTEL_TRACES_SAMPLER=parentbased_traceidratio` and `OTEL_TRACES_SAMPLER_ARG=0.1`, meaning 10% of requests will be traced. This is appropriate for development and demo environments. For high-traffic production deployments, consider reducing this to 1-5%.

4. **Service names** — The scheduler reports as `gateway-api-inference-extension` and vLLM reports as `vllm-decode`. These names appear in the Jaeger UI as separate services, allowing you to filter and correlate spans across the request lifecycle.

5. **OTLP endpoint** — Both components point at `otel-collector:4317`, the service created by the OpenTelemetry operator in step 5. This is the gRPC endpoint of the OTel Collector.

## Verifying Distributed Tracing

### Sending test traffic

To verify that traces are flowing end-to-end, port-forward the gateway service and send a few requests:

```bash
# Find the gateway service name
GATEWAY_SVC=$(oc get svc -n distributed-tracing -l gateway.networking.k8s.io/gateway-name -o jsonpath='{.items[0].metadata.name}')

# Port-forward in the background
kubectl port-forward -n distributed-tracing "svc/$GATEWAY_SVC" 9443:443 &
PF_PID=$!
sleep 3

# Send requests (at 10% sampling, 13 requests gives ~75% chance of at least one trace)
TOKEN=$(oc whoami -t)
for i in $(seq 1 13); do
    HTTP_CODE=$(curl -sk -o /dev/null -w '%{http_code}' \
        --noproxy localhost \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        https://localhost:9443/distributed-tracing/distributed-tracing-llama/v1/chat/completions \
        -d '{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"Hello, request '"$i"'"}],"max_tokens":50}')
    echo "Request $i/13 — HTTP $HTTP_CODE"
done

# Stop port-forward
kill $PF_PID
```

All requests should return `HTTP 200`.

### Viewing traces in the Jaeger UI

Get the Jaeger UI route:

```bash
oc get route -n distributed-tracing -l app.kubernetes.io/name=tempo
```

```console
NAME                     HOST/PORT                                                    PATH   SERVICES                 PORT     TERMINATION     WILDCARD
tempo-tracing-jaegerui   tempo-tracing-jaegerui-distributed-tracing.apps.example.com          tempo-tracing-jaegerui   16686    edge/Redirect   None
```

Open the `HOST/PORT` URL in your browser. In the Jaeger UI:

1. Select a service from the **Service** dropdown (e.g., `vllm-decode` or `gateway-api-inference-extension`).
2. Click **Find Traces**.
3. Click on a trace to expand its span timeline.

You should see spans from both the inference scheduler and vLLM, showing the full request path through the llm-d stack.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Jaeger UI shows 0 services | OTel Collector can't reach Tempo | Check collector logs: `oc logs -l app.kubernetes.io/name=otel-collector -n distributed-tracing`. If you see `connection refused` errors, restart the collector pod: `oc delete pod -l app.kubernetes.io/name=otel-collector -n distributed-tracing` |
| vLLM pod crashes with `AttributeError: type object 'Any' has no attribute 'SERVER'` | Missing OpenTelemetry SDK packages in the vLLM image | The RHAIIS image may not ship the full OpenTelemetry SDK. Use an init container to install `opentelemetry-sdk` and `opentelemetry-exporter-otlp` into a shared volume, and set `PYTHONPATH` to include it. See the [workaround section](#workaround-missing-opentelemetry-sdk-in-the-vllm-image) below |
| `missing tenant header` in Jaeger UI | Multi-tenancy is enabled on the Tempo instance | Disable multi-tenancy in the `TempoMonolithic` spec, or access Tempo directly bypassing the gateway |
| Traces appear for `vllm-decode` but not `gateway-api-inference-extension` (or vice versa) | Only one component has tracing configured | Verify both the scheduler (`--tracing=true`) and vLLM (`--otlp-traces-endpoint`) have tracing enabled in the LLMISVC spec |
| `GatewayPreconditionNotMet` on LLMISVC | llmisvc controller started before Connectivity Link CRDs were available | Restart the llmisvc controller pod: `oc rollout restart deployment llmisvc-controller-manager -n redhat-ods-applications` |
| Gateway shows `PROGRAMMED=False` | Service type is `LoadBalancer` but no LB controller exists | Change the gateway ConfigMap service type to `ClusterIP` |

### Workaround: missing OpenTelemetry SDK in the vLLM image

If the RHAIIS vLLM image does not include the full OpenTelemetry SDK (causing the `SpanKind.SERVER` crash described above), you can work around this by adding an init container that installs the missing packages into a shared volume:

```yaml
# Add to spec.template in the LLMInferenceService:
  template:
    initContainers:
      - name: install-otel-sdk
        image: registry.redhat.io/rhaii-early-access/vllm-cuda-rhel9:3.4.0-ea.2-1774939203
        command: ["pip", "install", "--target=/otel-packages", "--quiet",
                  "opentelemetry-sdk", "opentelemetry-exporter-otlp"]
        volumeMounts:
          - name: otel-packages
            mountPath: /otel-packages
    volumes:
      - name: otel-packages
        emptyDir: {}
    containers:
      - name: main
        # ... existing container spec ...
        volumeMounts:
          - name: otel-packages
            mountPath: /otel-packages
        env:
          - name: PYTHONPATH
            value: "/otel-packages"
          # ... other env vars ...
```

This installs `opentelemetry-sdk` and `opentelemetry-exporter-otlp` into an `emptyDir` volume that is shared with the main container via `PYTHONPATH`. The init container does not require a GPU and uses the same base image for Python compatibility.

**NOTE:** This workaround requires network access to PyPI from the init container. It will not work in air-gapped environments. For air-gapped clusters, build a custom vLLM image that includes the OpenTelemetry packages.
