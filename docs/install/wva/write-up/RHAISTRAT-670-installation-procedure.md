# Enable the Workload Variant Autoscaler (WVA) for llm-d deployments

You can enable intelligent autoscaling for your llm-d model deployments by configuring the workload variant autoscaler (WVA). The WVA controller is automatically deployed when the {productname-short} Operator is installed. After you configure autoscaling in the LLMInferenceService custom resource, the WVA automatically adjusts the replica count of your model server based on real-time inference traffic and AI accelerator capacity.

## Prerequisites

**NOTE**: Many of these dependencies are not specific to Autoscaling. We call out dependencies of installing and using the llm-d stack through {productname-long}

- You have an OpenShift cluster on version `4.20` or later.

- You have installed the OpenShift CLI (`oc`).

- (OPTIONAL) You have `jq` installed for parsing JSON bodies, and `yq` installed for selecting sub sections of our yaml resources. This is used in the [Verifying The Autoscaling Behaviour](./RHAISTRAT-670-installation-procedure.md#verifying-the-autoscaling-behaviour) section to parse request bodies in a clean way.

- You have logged in as a user with cluster-admin privileges.

- Compatible AI accelerators are available in the cluster. This technically should work with CPU inferencing as well but it has not been tested for Dev Preview.

- OpenShift User Workload Monitoring (UWM) is enabled in the cluster for Prometheus metrics collection. For instructions on how to do this, refer to, [the official Red Hat docs](https://docs.redhat.com/en/documentation/monitoring_stack_for_red_hat_openshift/4.20/html/configuring_user_workload_monitoring/index).

- You have installed the `custom-metrics-autoscaler` operator from OperatorHub. For more information on how to do this, refer to the [Red Hat official documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/nodes/automatically-scaling-pods-with-the-custom-metrics-autoscaler-operator) on installing it.

- You have installed the `Red Hat Connectivity Link` operator from OperatorHub. For more information on how to do this, refer to the [Red Hat official documentation](https://docs.redhat.com/en/documentation/red_hat_connectivity_link/1.3/html/installing_on_openshift_container_platform/index) on installing it.

- You have the `Red Hat OpenShift Service Mesh 3` operator installed in your cluster. You should have this by default on any OpenShift cluster version `4.20` or later.

- You have installed {productname-long} {vernum}.

- A `DataScienceClusterInitialization` (DSCI) and `DataScienceCluster` (DSC) exist in your cluster, enabling the `workload-variant-autoscaler-controller-manager`, `llmisvc-controller-manager` and `kserve-controller-manager`. The `DataScienceClusterInitialization` gets created by the Red Hat OpenShift-AI operator out of the box for you. This is an sample exerpt from the `DataScienceCluster` manifest that enables the WVA controller:

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
      managementState: "Managed" # Create KServe and LLMISVC controller managers
      nim:
        managementState: "Managed"
      rawDeploymentServiceConfig: "Headed"
      wva:
        managementState: "Managed" # Create workload-variant-autoscaler controller manager
    ... # Other DSC components as desired
```

- No other `LLMInferenceService`s exist in the namespace you intend to deploy in (each namespace contains only one llm-d inference stack).

## Procedure

### Verifying our Pre-requisites

- First, lets verify that we have the following controllers exist in our cluster:

```yaml
oc get pods -l app.kubernetes.io/name=kserve-controller-manager -n redhat-ods-applications
NAME                                        READY   STATUS    RESTARTS   AGE
kserve-controller-manager-b99f6f67d-ctdgk   1/1     Running   0          38m

oc get pods -l app.kubernetes.io/name=llmisvc-controller-manager -n redhat-ods-applications
NAME                                          READY   STATUS    RESTARTS   AGE
llmisvc-controller-manager-698f84cc75-nrt5d   1/1     Running   0          38m

oc get pods -l app.kubernetes.io/name=workload-variant-autoscaler -n redhat-ods-applications
NAME                                                              READY   STATUS    RESTARTS   AGE
workload-variant-autoscaler-controller-manager-85b8d895cd-v5gs7   1/1     Running   0          38m
```

- We can also verify our configurations:

```yaml
oc get cm -n redhat-ods-applications
NAME                                                        DATA   AGE
dashboard-redirect-config                                   1      4h26m
inferenceservice-config                                     19     4h21m
kserve-parameters                                           13     4h21m
kube-root-ca.crt                                            1      4h27m
odh-kserve-custom-ca-bundle                                 1      4h22m
odh-model-controller-parameters                             15     4h22m
odh-segment-key-config                                      1      4h27m
odh-trusted-ca-bundle                                       2      4h27m
OpenShift-service-ca.crt                                    1      4h27m
workload-variant-autoscaler-saturation-scaling-config       1      4h22m
workload-variant-autoscaler-wva-variantautoscaling-config   13     4h22m
```

Of these the only config we should be touching that is related to autoscaling is the `workload-variant-autoscaler-saturation-scaling-config` `ConfigMap` which we will be editing later in this guide. For now we can verify that its contents is consistent with what we expect to see for default scaling configurations for WVA:

```bash
oc get cm workload-variant-autoscaler-saturation-scaling-config -n redhat-ods-applications -o yaml | yq .data.default

kvCacheThreshold: 0.80
queueLengthThreshold: 5
kvSpareTrigger: 0.1
queueSpareTrigger: 3
# Enable GPU limiter to constrain scaling based on available cluster resources
# When true, scale-up decisions are limited by available GPU capacity
enableLimiter: false
```

### Granting KEDA permissions to read from OpenShift Monitoring

The Custom Metrics Autoscaler operator does not automatically have permission to read metrics from the OpenShift-monitoring or OpenShift-user-workload-monitoring stack because OpenShift restricts access to cluster monitoring APIs. This is done by design, wheere KEDA makes no assumptions about where it gets its metrics from, even when deploying on OpenShift. Access to Prometheus and Thanos endpoints requires explicit RBAC permissions such as the cluster-monitoring-view role. There are two steps to this, **authorization** and **authentication**, lets start with the former. 

#### Authorizing KEDA to OCP UWM metrics

To grant KEDA authorizaiton to see view metrics we will need a `ServiceAccount`, `Secret` to back the service account as a token and `ClusterRoleBinding` granting `cluster-monitoring-view` to our `ServiceAccount`.

You can do this by applying the following yaml:

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: keda-metrics-reader
  namespace: OpenShift-keda
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: keda-metrics-reader-monitoring
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-monitoring-view
subjects:
  - kind: ServiceAccount
    name: keda-metrics-reader
    namespace: OpenShift-keda
---
apiVersion: v1
kind: Secret
metadata:
  name: keda-metrics-reader-token
  namespace: OpenShift-keda
  annotations:
    kubernetes.io/service-account.name: keda-metrics-reader
type: kubernetes.io/service-account-token
EOF
```

After this we should see confimation that our three resources were created:

```console
serviceaccount/keda-metrics-reader created
clusterrolebinding.rbac.authorization.k8s.io/keda-metrics-reader-monitoring created
secret/keda-metrics-reader-token created
```

#### Allowing KEDA to Authenticate against OpenShift user workload monitoring

To give KEDA the proper permissions, we need to create a `ClusterTriggerAuthentication`, which will reference the `keda-metrics-reader-token` `Secret` acting as a Token for the `keda-metrics-reader` `ServiceAccount`, both of which we created in the previous step. To create our `ClusterTriggerAuthentication` do the following:

```bash
oc apply -f - <<'EOF'
apiVersion: keda.sh/v1alpha1
kind: ClusterTriggerAuthentication
metadata:
  name: ai-inference-keda-thanos
spec:
  secretTargetRef:
    - parameter: bearerToken
      name: keda-metrics-reader-token
      key: token
    - parameter: ca
      name: keda-metrics-reader-token
      key: ca.crt
EOF
```

We should receive confirmation from the server that we have created our `ClusterTriggerAuthentication`:

```console
clustertriggerauthentication.keda.sh/ai-inference-keda-thanos created
```

Now KEDA will be able pull metrics from the OpenShift User Workload Monitoring stack.

#### Patching the `inferenceservice-config` to use OpenShift User Workload Monitoring

In the previous steps we gave KEDA the authorization and authentication it needs to query Thanos Querier in OpenShift User Workload Monitoring. But KEDA doesn't know where to find those metrics or which credentials to use — that information comes from the llmisvc controller, which reads its autoscaling configuration from the `inferenceservice-config` ConfigMap.

By default, this ConfigMap has no autoscaling configuration set. We need to patch it to tell the llmisvc controller:

  1. **Where** to query metrics — the Thanos Querier URL
  2. **How** to authenticate — using the `ClusterTriggerAuthentication` resource
     (`ai-inference-keda-thanos`) we created in the previous step

To do this, we can run the a `kubectl patch` command, and then we need to cycle our `llmisvc-controller-manager` deployment, so that it can pickup our configuration changes. You can do so with the following:

```bash
oc patch configmap inferenceservice-config -n redhat-ods-applications \
    --type=json -p '[{"op":"replace","path":"/data/autoscaling-wva-controller-config","value":"{\"prometheus\":{\"url\":\"https://thanos-querier.OpenShift-monitoring.svc.cluster.local:9091\",\"authModes\":\"bearer\",\"triggerAuthName\":\"ai-inference-keda-thanos\",\"triggerAuthKind\":\"ClusterTriggerAuthentication\"}}"}]'
oc rollout restart deployment llmisvc-controller-manager -n redhat-ods-applications
```

This should result in the following logs:

```console
configmap/inferenceservice-config patched
deployment.apps/llmisvc-controller-manager restarted
```

Now, when the `LLMISVC` gets created with autoscaling configurations, the `llmisvc-controller-manager` will create a KEDA `ScaledObject` with these settings embedded so KEDA knows how to connect to Thanos and authenticate its metric queries.

### Creating an Inference Stack with Autoscaling enabled

Now we are ready to start applying all of our resources related to our `LLMInferenceService`. 

#### Creating the Namespace

Because **we only allow for one inference stack per namesapce** at this time, we will create the `autoscaling-example` namespace, although this should work for any namespace provided you adjust the manifests accordingly.

```bash
oc create ns autoscaling-example && oc project autoscaling-example
```

`oc` should respond by telling us our namespace was created and we are using that project:

```console
namespace/autoscaling-example created
Now using project "autoscaling-example" on server "https://api.qfpda-by6c4-2cb.oiah.p3.openshiftapps.com:443".
```

#### Creating the Gateway

Next were going to create a gateway that will use TLS using OpenShift Service Mesh. As described in the [pre-requisites section](./RHAISTRAT-670-installation-procedure.md#prerequisites), this should come pre-installed with OpenShift 4.20+. To create our `Gateway`, we will create the following `Gateway` and `ConfigMap` manifests:

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: autoscaling-example-gateway-config
  namespace: autoscaling-example
data:
  service: |
    metadata:
      annotations:
        service.beta.OpenShift.io/serving-cert-secret-name: "autoscaling-example-gateway-tls"
    spec:
      type: ClusterIP
  deployment: |
    spec:
      template:
        spec:
          containers:
            - name: istio-proxy
              resources:
                limits:
                  cpu: "16"
                  memory: 16Gi
                requests:
                  cpu: "4"
                  memory: 4Gi
EOF
oc apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: autoscaling-example-gateway
  namespace: autoscaling-example
spec:
  gatewayClassName: data-science-gateway-class
  infrastructure:
    parametersRef:
      group: ""
      kind: ConfigMap
      name: autoscaling-example-gateway-config
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
        name: autoscaling-example-gateway-tls
      mode: Terminate
EOF
```

This should result in the following confirmation message that our configmap and gateway were created:

```console
configmap/autoscaling-example-gateway-config created
gateway.gateway.networking.k8s.io/autoscaling-example-gateway created
```

Some things to note about this gateway setup:
  1. We have increased the default resources of the gateway because we will be attempting to reproduce a scaling event later on with significant load generation.
  2. We have opted for a `ClusterIP` service type here because not all OCP flavours have `LoadBalancer` service type integration. `ROSA` for instance or OpenShift on IBM cloud do, while others do not. For this reason this demo is most easily reproduceable with port-forwarding for local requests, and doing load gen through a pod in the cluster against the internal service address (`port-forwards` fall down and significant concurrency).

#### Creating and Analyzing the LLMISVC

Next we will create our `LLMISVC` with autoscaling configurations enabled, as well as briefly discuss how you might alter this to fit your use case. You can apply our `LLMISVC` as so:

```bash
oc apply -f - <<'EOF'
apiVersion: serving.kserve.io/v1alpha2
kind: LLMInferenceService
metadata:
  name: autoscaling-example-llama
  namespace: autoscaling-example
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8000"
    prometheus.io/path: "/metrics"
    security.opendatahub.io/enable-auth: 'false' # disable llmisvc auth for this example
spec:
  router:
    scheduler: {}
    route: {}
    gateway:
      refs: # Reference the gateway we created
      - name: autoscaling-example-gateway
        namespace: autoscaling-example
  model:
    uri: hf://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
  labels:
    inference.optimization/acceleratorName: H100 # If using Accelerators, use the Name of your Accelerator
  scaling:
    minReplicas: 1
    maxReplicas: 5
    wva:
      keda:
        pollingInterval: 5
        cooldownPeriod: 30
  template:
    containers:
      - name: main
        image: quay.io/aipcc/rhaiis/cuda-ubi9:3.4.0-ea.2 # If using Accelerators, use the corresponding inference-image
        resources:
          limits:
            cpu: '4'
            memory: 32Gi
            nvidia.com/gpu: 1
          requests:
            cpu: '2'
            memory: 16Gi
            nvidia.com/gpu: 1
        startupProbe:
          httpGet:
            path: /health
            port: 8000
            scheme: HTTPS
          failureThreshold: 60
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8000
            scheme: HTTPS
        livenessProbe:
          httpGet:
            path: /health
            port: 8000
            scheme: HTTPS
EOF
```

Upon successful creation of our LLMISVC we should see the following:
```console
llminferenceservice.serving.kserve.io/autoscaling-example-llama created
```

Some important pieces to note about this:

1. `.metadata.annotations.security.opendatahub.io/enable-auth='false'` - This is important. In RHOAI user authentication and rate limiting are done through the Red Hat Connectivity Link operator. In this example we will need to bypass this, but production deployments will not. This is because the amount of load we need to generate would far surpass the amount of load you would ever see **from a single user**, to the point where we would hope rate limiting and quota would kick in. For this reason we are disabling the authentication / ratelimiting to request through the `LLMISVC` + `Gateway`. However as stated before, production deployments would have their load generated across many users, none of which would be be violating the rate limit policy at a per user level.
2. `.spec.router.gateway.refs` - This is how we tell the LLMISVC to use our pre-created `Gateway`
3. `.spec.labels.inference.optimization/acceleratorName` - Tells the Workload Variant Autoscaler what type of accelerator this workload uses (e.g., `A100`, `H100`, `cpu`). WVA uses this to group variants by hardware type and look up accelerator-specific performance data when making scaling decisions. There is no fixed list of valid values — use whatever matches your hardware for semantic identification. If omitted, it defaults to `unknown`.
4. `.spec.scaling` - This is where we specify user-facing scaling configurations at a per `LLMISVC` (AKA per inference stack) level. Not to be confused with the configurations from the  `workload-variant-autoscaler-saturation-scaling-config` `ConfigMap`, which represents the default scaling configurations that the `workload-variant-autoscaler` will use to determine at what threshold it decides to scale your inference workloads up or down. For a list of what can be configured here see below (@AIDIN link to the docs outside procedure).
5. `.spec.template.containers[0].image` - Different inference images are used for different accelerators. 
6. `.spec.template.containers[0].args`, specifically `--ssl-certfile` and `--ssl-keyfile` - This example runs entirely over TLS. These cert and key files are needed to enable the inference container(s) to do this.
7. `.spec.template.containers[0].resources` (`requests` and `limits`) - This example is done with a single replica and Tensor Parallelism one. Adjust the resources as needed for more GPUs and adjust add the tensor parallel flags to the LLMISVC ENV var: `VLLM_ADDITIONAL_ARGS`, ex:
```yaml
env:
  - name: VLLM_ADDITIONAL_ARGS
    value: "--tensor-parallel 2"
```
    - **NOTE:** The same thing (adding device requests to the `resources` and `limits` sections) applies for accelerated networking devices like RoCE, or Infiniband, although autoscaling internode workloads through `LeaderWorkerSet` falls outside support at this time.
8. `.spec.template.containers[0]` Probe schemes (`startupProbe`, `readinessProbe`, and `livenessProbe`) - As describe above this example uses TLS end to end, if you do not ensure you checks use the TLS scheme it will fail to hit the endpoints, and thus declare the pods ready or health.

#### Metric relabelling mapping

By default vLLM exposes metrics to the `vllm` namespace, but in Red Hat OpenShift AI (RHOAI), we re-map these to the `kserve_vllm` namespace. This means the Workload Variant Autoscaler will only be looking for the kserve namespace instances of these metrics. To enable the WVA to pick up these metrics we create a `PrometheusRule` responsible for creating aliases for them. You will need to create the following manifest:

```bash
oc apply -f - <<'EOF'
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: vllm-metrics-alias
  namespace: autoscaling-example
  labels:
    monitoring.opendatahub.io/scrape: "true"
spec:
  groups:
  - name: vllm-metric-aliases
    interval: 15s
    rules:
    - record: vllm:kv_cache_usage_perc
      expr: kserve_vllm:kv_cache_usage_perc
    - record: vllm:num_requests_waiting
      expr: kserve_vllm:num_requests_waiting
    - record: vllm:num_requests_running
      expr: kserve_vllm:num_requests_running
    - record: vllm:cache_config_info
      expr: kserve_vllm:cache_config_info
    - record: vllm:request_success_total
      expr: kserve_vllm:request_success_total
    - record: vllm:request_generation_tokens_sum
      expr: kserve_vllm:request_generation_tokens_sum
    - record: vllm:request_generation_tokens_count
      expr: kserve_vllm:request_generation_tokens_count
    - record: vllm:request_prompt_tokens_sum
      expr: kserve_vllm:request_prompt_tokens_sum
    - record: vllm:request_prompt_tokens_count
      expr: kserve_vllm:request_prompt_tokens_count
    - record: vllm:time_to_first_token_seconds_sum
      expr: kserve_vllm:time_to_first_token_seconds_sum
    - record: vllm:time_to_first_token_seconds_count
      expr: kserve_vllm:time_to_first_token_seconds_count
    - record: vllm:time_per_output_token_seconds_sum
      expr: kserve_vllm:time_per_output_token_seconds_sum
    - record: vllm:time_per_output_token_seconds_count
      expr: kserve_vllm:time_per_output_token_seconds_count
    - record: vllm:prefix_cache_hits
      expr: kserve_vllm:prefix_cache_hits
    - record: vllm:prefix_cache_queries
      expr: kserve_vllm:prefix_cache_queries
EOF
```

If everything goes right, you should receive confirmation that your `PrometheusRule` has been created:

```console
prometheusrule.monitoring.coreos.com/vllm-metrics-alias created
```

## Verifying the Autoscaling Behaviour

Breaking the verification of Autoscaling down into three separate steps helps to ensure our functionality while providing useful guards for debugging in case something has gone wrong. Lets examine if our system can do the following:

1. Are our Inference Server metrics showing up in Prometheus?
2. Can the WVA emit metrics back to Prometheus to be consumed by KEDA?
3. Can we actually observe an autoscaling event (E2E test)?

### Verify our Component metrics show up in Prometheus

We can start by ensuring that our inputs to the WVA - the inference-server metrics - show up in Prometheus.

```bash
TOKEN=$(oc whoami -t)
THANOS=$(oc get route thanos-querier -n OpenShift-monitoring -o jsonpath='{.spec.host}')
for m in num_requests_running num_requests_waiting kv_cache_usage_perc; do
    curl -sk -G -H "Authorization: Bearer $TOKEN" "https://$THANOS/api/v1/query" \
      --data-urlencode "query=vllm:${m}{namespace=\"autoscaling-example\"}" | \
      jq '.data.result[0]'
done
```

You should see three separate JSON payloads for each:

```json
{
  "metric": {
    "__name__": "vllm:num_requests_running",
    "container": "main",
    "endpoint": "8000",
    "engine": "0",
    "instance": "10.130.3.118:8000",
    "job": "autoscaling-example/kserve-llm-isvc-vllm-engine",
    "llm_isvc_component": "workload",
    "llm_isvc_name": "autoscaling-example-llama",
    "llm_isvc_role": "both",
    "model_name": "Qwen/Qwen2.5-7B-Instruct",
    "namespace": "autoscaling-example",
    "pod": "autoscaling-example-llama-kserve-56b64d69d-b297x",
    "prometheus": "OpenShift-user-workload-monitoring/user-workload"
  },
  "value": [
    1775002422.238,
    "0"
  ]
}
{
  "metric": {
    "__name__": "vllm:num_requests_waiting",
    "container": "main",
    "endpoint": "8000",
    "engine": "0",
    "instance": "10.130.3.118:8000",
    "job": "autoscaling-example/kserve-llm-isvc-vllm-engine",
    "llm_isvc_component": "workload",
    "llm_isvc_name": "autoscaling-example-llama",
    "llm_isvc_role": "both",
    "model_name": "Qwen/Qwen2.5-7B-Instruct",
    "namespace": "autoscaling-example",
    "pod": "autoscaling-example-llama-kserve-56b64d69d-b297x",
    "prometheus": "OpenShift-user-workload-monitoring/user-workload"
  },
  "value": [
    1775002422.711,
    "0"
  ]
}
{
  "metric": {
    "__name__": "vllm:kv_cache_usage_perc",
    "container": "main",
    "endpoint": "8000",
    "engine": "0",
    "instance": "10.130.3.118:8000",
    "job": "autoscaling-example/kserve-llm-isvc-vllm-engine",
    "llm_isvc_component": "workload",
    "llm_isvc_name": "autoscaling-example-llama",
    "llm_isvc_role": "both",
    "model_name": "Qwen/Qwen2.5-7B-Instruct",
    "namespace": "autoscaling-example",
    "pod": "autoscaling-example-llama-kserve-56b64d69d-b297x",
    "prometheus": "OpenShift-user-workload-monitoring/user-workload"
  },
  "value": [
    1775002423.191,
    "0"
  ]
}
```

**NOTE:** Currently the WVA does not currently use metrics from the inference scheduler but this is coming soon.


### Verify WVA is emitting metrics back to Prometheus

The three metrics were looking for are `wva_current_replicas`, `wva_desired_replicas`, and `wva_desired_ratio`. We can grab those with the following

```bash
TOKEN=$(oc whoami -t)
THANOS=$(oc get route thanos-querier -n OpenShift-monitoring -o jsonpath='{.spec.host}')
for m in wva_desired_replicas wva_current_replicas wva_desired_ratio; do
  curl -sk -G -H "Authorization: Bearer $TOKEN" "https://$THANOS/api/v1/query" \
    --data-urlencode "query=${m}{exported_namespace=\"autoscaling-example\"}" \
    | jq '.data.result[0]'
done
```

You should see the following:

```json
{
  "metric": {
    "__name__": "wva_desired_replicas",
    "accelerator_type": "H100",
    "endpoint": "https",
    "exported_namespace": "autoscaling-example",
    "instance": "10.130.3.99:8443",
    "job": "workload-variant-autoscaler-controller-manager-metrics-service",
    "namespace": "redhat-ods-applications",
    "pod": "workload-variant-autoscaler-controller-manager-774bc99447-x8l8d",
    "prometheus": "OpenShift-user-workload-monitoring/user-workload",
    "service": "workload-variant-autoscaler-controller-manager-metrics-service",
    "variant_name": "autoscaling-example-llama-kserve-va"
  },
  "value": [
    1775002469.694,
    "1"
  ]
}
{
  "metric": {
    "__name__": "wva_current_replicas",
    "accelerator_type": "H100",
    "endpoint": "https",
    "exported_namespace": "autoscaling-example",
    "instance": "10.130.3.99:8443",
    "job": "workload-variant-autoscaler-controller-manager-metrics-service",
    "namespace": "redhat-ods-applications",
    "pod": "workload-variant-autoscaler-controller-manager-774bc99447-x8l8d",
    "prometheus": "OpenShift-user-workload-monitoring/user-workload",
    "service": "workload-variant-autoscaler-controller-manager-metrics-service",
    "variant_name": "autoscaling-example-llama-kserve-va"
  },
  "value": [
    1775002470.133,
    "1"
  ]
}
{
  "metric": {
    "__name__": "wva_desired_ratio",
    "accelerator_type": "H100",
    "endpoint": "https",
    "exported_namespace": "autoscaling-example",
    "instance": "10.130.3.99:8443",
    "job": "workload-variant-autoscaler-controller-manager-metrics-service",
    "namespace": "redhat-ods-applications",
    "pod": "workload-variant-autoscaler-controller-manager-774bc99447-x8l8d",
    "prometheus": "OpenShift-user-workload-monitoring/user-workload",
    "service": "workload-variant-autoscaler-controller-manager-metrics-service",
    "variant_name": "autoscaling-example-llama-kserve-va"
  },
  "value": [
    1775002470.557,
    "1"
  ]
}
```

At this point you should have the following resources that got created from your `LLMISVC`:

```bash
k get va autoscaling-example-llama-kserve-va
NAME                                  TARGET                             MODEL                      OPTIMIZED   METRICSREADY   AGE
autoscaling-example-llama-kserve-va   autoscaling-example-llama-kserve   Qwen/Qwen2.5-7B-Instruct   1           True           81m

k get scaledObject -n autoscaling-example
NAME                                    SCALETARGETKIND      SCALETARGETNAME                    MIN   MAX   READY   ACTIVE   FALLBACK   PAUSED   TRIGGERS     AUTHENTICATIONS            AGE
autoscaling-example-llama-kserve-keda   apps/v1.Deployment   autoscaling-example-llama-kserve   1     5     True    True     Unknown    False    prometheus   ai-inference-keda-thanos   83m
```

It should be noted for your `VariantAutoscaling` it should show `METRICSREADY=True` and your scaledObject should be showing as `READY=true`. Once the WVA controller starts reconciling the `scaledObject` it should also become `ACTIVE=True`. If you are not seeing that, your first deubgging steps should be to `describe` these resources and check their `Status` sections of their yaml.

### Verifying the inference worker gets Autoscaled

This is the fun part; we get to watch our inference workers scale before our very eyes! In this example were going to make this experiment easier on ourselves by setting the `kvCacheThreshold` key in the `workload-variant-autoscaler-saturation-scaling-config ConfigMap` in the `redhat-ods-applications` namespace to `0.10`. We are doing this because generating enough load to see a scaling event is actually harder than one might think, this was tested on an H100 and it was handling enough load to shift the challenge to the benchmarking client side. We also chose to keep this as a script rather than an official benchmark tool for easy integration with testing with auth enabled. We have temporarily disabled it to showcase this functionality because the rate limiting is done at a per-user level.

**NOTE**: Scaling + Load chosen in the script is HIGHLY dependent on the accelerators in question. This exmaple is taken using H100s.

```bash
#!/bin/bash
# Test WVA autoscaling by running a load generator pod inside the cluster
# and watching the deployment scale up.
#
# Usage:
#   ./09c-test-autoscaling.sh [namespace] [isvc-name] [concurrency] [requests]
set -euo pipefail

NS="${1:-autoscaling-example}"
ISVC="${2:-autoscaling-example-llama}"
CONCURRENCY="${3:-200}"
REQUESTS="${4:-5000}"
POD_NAME="load-test-$(date +%s)"
CM_NAME="script-${POD_NAME}"

# Resolve gateway service FQDN
GATEWAY_SVC=$(oc get svc -n "$NS" -l gateway.networking.k8s.io/gateway-name \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if [ -z "$GATEWAY_SVC" ]; then
  echo "ERROR: No gateway service found in namespace $NS"
  exit 1
fi
URL="https://${GATEWAY_SVC}.${NS}.svc.cluster.local/${NS}/${ISVC}/v1/chat/completions"

echo "=== WVA Autoscaling Test ==="
echo "URL:         $URL"
echo "Concurrency: $CONCURRENCY"
echo "Requests:    $REQUESTS"
echo ""
oc get deployment "${ISVC}-kserve" -n "$NS" \
  -o jsonpath='Initial state: {.spec.replicas}/{.status.readyReplicas} ready' && echo ""
echo ""

# Cleanup on exit
cleanup() {
  echo ""
  echo "Cleaning up..."
  oc delete pod "$POD_NAME" -n "$NS" --ignore-not-found --wait=false 2>/dev/null
  oc delete configmap "$CM_NAME" -n "$NS" --ignore-not-found 2>/dev/null
}
trap cleanup EXIT INT TERM

# Load script that runs inside the cluster pod.
# Uses a wrapper script so xargs passes arguments cleanly.
read -r -d '' LOAD_SCRIPT << 'EOF' || true
#!/bin/sh
set -e
URL="$1"; CONCURRENCY="$2"; REQUESTS="$3"

# Wrapper script for xargs — each invocation sends one request
cat > /tmp/req.sh << 'REQEOF'
#!/bin/sh
curl -sk --max-time 600 "$1" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"Qwen/Qwen2.5-7B-Instruct\",\"messages\":[{\"role\":\"user\",\"content\":\"Request $2. Write a detailed essay about topic $2 covering history, analysis, and predictions.\"}],\"max_tokens\":2048}" \
  -o /dev/null -w "req=$2 status=%{http_code} time=%{time_total}s\n"
REQEOF
chmod +x /tmp/req.sh

# Verify connectivity
echo "Smoke test..."
STATUS=$(curl -sk --max-time 30 "$URL" \
  -H "Content-Type: application/json" \
  -d '{"model":"Qwen/Qwen2.5-7B-Instruct","messages":[{"role":"user","content":"Hi"}],"max_tokens":5}' \
  -o /dev/null -w "%{http_code}")
echo "Status: $STATUS"
if [ "$STATUS" != "200" ]; then
  echo "ERROR: Smoke test failed (HTTP $STATUS)"
  exit 1
fi

echo "Sending $REQUESTS requests ($CONCURRENCY concurrent)..."
seq 1 "$REQUESTS" | xargs -P "$CONCURRENCY" -I{} /tmp/req.sh "$URL" {}
echo "Done."
EOF

# Deploy load generator
oc create configmap "$CM_NAME" -n "$NS" --from-literal=load.sh="$LOAD_SCRIPT"

cat <<MANIFEST | oc apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME
  namespace: $NS
spec:
  restartPolicy: Never
  containers:
  - name: load
    image: curlimages/curl
    command: ["sh", "/scripts/load.sh"]
    args: ["$URL", "$CONCURRENCY", "$REQUESTS"]
    resources:
      requests: { cpu: "4", memory: "4Gi" }
      limits:   { cpu: "8", memory: "8Gi" }
    volumeMounts:
    - { name: script, mountPath: /scripts }
  volumes:
  - name: script
    configMap: { name: $CM_NAME, defaultMode: 0755 }
MANIFEST

echo "Waiting for pod..."
oc wait --for=condition=Ready pod/"$POD_NAME" -n "$NS" --timeout=120s 2>/dev/null || true
sleep 2

POD_PHASE=$(oc get pod "$POD_NAME" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null)
if [ "$POD_PHASE" = "Failed" ]; then
  echo "ERROR: Pod failed:"
  oc logs "$POD_NAME" -n "$NS" 2>/dev/null
  exit 1
fi

# Stream logs in background, watch replicas in foreground
oc logs -f "$POD_NAME" -n "$NS" 2>/dev/null &
LOGS_PID=$!

echo ""
echo "--- Watching replicas (Ctrl+C to stop) ---"
for _ in $(seq 1 120); do
  replicas=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.spec.replicas}' 2>/dev/null)
  ready=$(oc get deployment "${ISVC}-kserve" -n "$NS" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  echo "  [$(date +%H:%M:%S)] Replicas: ${replicas:-?} (${ready:-0} ready)"
  [ "${replicas:-1}" -gt 1 ] && echo "  ** Scale-up detected! **"
  oc get pod "$POD_NAME" -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null \
    | grep -q "Succeeded\|Failed" && echo "  Load pod finished." && break
  sleep 5
done

kill "$LOGS_PID" 2>/dev/null || true
wait "$LOGS_PID" 2>/dev/null || true
```

This script does the following:
  - Resolves the gateway service inside the cluster using the LLMISVC namespace and gateway labels
  - Deploys a load generator pod (curlimages/curl) with high resource limits to avoid process/memory throttling
  - Sends thousands of concurrent requests with unique prompts and long token generation (2048 max_tokens) to saturate the vLLM KV cache
  past the WVA scaling threshold
  - Monitors replica count every 5 seconds, reporting scale-up events as WVA responds to the load

This design was chosen rather than a more robust benchmarking tool to because when auth is re-enabled after the orrigional exmaple, its simple enough to extend curl requests with a Bearer token generated from `$(oc whoami -t)`, while this is not trivial from modern benchmarking tools. It was additional designed to run the cluster because `port-forwards` were not designed for benchmarking and drop requests at high concurrency.

The following is some sample output from the benchmarking script:

```console
=== WVA Autoscaling Test ===
URL:         https://autoscaling-example-gateway-data-science-gateway-class.autoscaling-example.svc.cluster.local/autoscaling-example/autoscaling-example-llama/v1/chat/completions
Concurrency: 200
Requests:    5000

Initial state: 1/1 ready

configmap/script-load-test-1775003391 created
pod/load-test-1775003391 created
Waiting for pod...
pod/load-test-1775003391 condition met

--- Watching replicas (Ctrl+C to stop) ---
Smoke test...
Status: 200
Sending 5000 requests (200 concurrent)...
req=39 status=200 time=1.189486s
req=123 status=200 time=1.252105s
req=115 status=200 time=1.296632s
req=5 status=200 time=1.432697s
req=4 status=200 time=1.542140s
req=3 status=200 time=1.638765s
req=113 status=200 time=1.666453s
req=111 status=200 time=2.052361s
req=122 status=200 time=2.588750s
  [17:30:03] Replicas: 1 (1 ready)
  [17:30:10] Replicas: 1 (1 ready)
req=163 status=200 time=11.189949s
req=1 status=200 time=16.619154s
  [17:30:17] Replicas: 1 (1 ready)
  [17:30:24] Replicas: 1 (1 ready)
  [17:30:30] Replicas: 1 (1 ready)
req=88 status=200 time=37.604920s
  [17:30:37] Replicas: 1 (1 ready)
  [17:30:44] Replicas: 1 (1 ready)
  [17:30:51] Replicas: 1 (1 ready)
  [17:30:58] Replicas: 2 (1 ready)
  ** Scale-up detected! **
  [17:31:06] Replicas: 2 (1 ready)
  ** Scale-up detected! **
  [17:31:13] Replicas: 2 (1 ready)
  ** Scale-up detected! **
req=36 status=200 time=75.716695s
  [17:31:20] Replicas: 2 (1 ready)
  ** Scale-up detected! **
req=29 status=200 time=86.615136s
  [17:31:27] Replicas: 2 (1 ready)
  ** Scale-up detected! **
...
```

**Note:** the repeated scale ups seen in the log are all from the same scaling event. The WVA employs a stair-step methodology to scaling, emposing a stabilization window on scaling events so that a traffic burst does not greedily attempt to occupy all your GPUs. However if the load continues to sustain past the stabilization window you may see additional scaling events. Additional for some time after the load ceases you will see the inference worker pods scale back down to 1.

During the load you might see:

```console
NAME                                                              READY   STATUS    RESTARTS      AGE
autoscaling-example-gateway-data-science-gateway-class-66c4qvpb   1/1     Running   0             81m
autoscaling-example-llama-kserve-56b64d69d-b297x                  1/1     Running   0             106m
autoscaling-example-llama-kserve-56b64d69d-kvvmc                  1/1     Running   0             10m
autoscaling-example-llama-kserve-56b64d69d-m78nz                  1/1     Running   0             7m1s
autoscaling-example-llama-kserve-router-scheduler-77cd5549p4w95   2/2     Running   0             106m
load-test-1775003391                                              1/1     Running   0             10m
```

And after it should scale back to :

```console
NAME                                                              READY   STATUS    RESTARTS      AGE
autoscaling-example-gateway-data-science-gateway-class-66c4qvpb   1/1     Running   0             81m
autoscaling-example-llama-kserve-56b64d69d-b297x                  1/1     Running   0             106m
autoscaling-example-llama-kserve-router-scheduler-77cd5549p4w95   2/2     Running   0             106m
load-test-1775003391                                              1/1     Running   0             10m
```
