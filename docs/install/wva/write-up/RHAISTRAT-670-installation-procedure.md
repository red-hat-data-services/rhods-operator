# Enable the Workload Variant Autoscaler (WVA) for llm-d deployments

You can enable intelligent autoscaling for your llm-d model deployments by configuring the workload variant autoscaler (WVA). The WVA controller is automatically deployed when the {productname-short} Operator is installed. After you configure autoscaling in the LLMInferenceService custom resource, the WVA automatically adjusts the replica count of your model server based on real-time inference traffic and AI accelerator capacity.

## Prerequisites

**NOTE**: Many of these dependencies are not specific to Autoscaling. We call out dependencies of installing and using the llm-d stack through {productname-long}

- You have an OpenShift cluster on version `4.20` or later.

- You have installed the OpenShift CLI (`oc`).

- (OPTIONAL) You have `jq` installed for parsing JSON bodies. This is used in the [Verifying The Autoscaling Behaviour](./RHAISTRAT-670-installation-procedure.md#verifying-the-autoscaling-behaviour) section to parse request bodies in a clean way.

- You have logged in as a user with cluster-admin privileges.

- Compatible AI accelerators are available in the cluster. This example uses the inference-simulator which runs on CPU but any accelerator supported by the inferencing image will work from the WVA perspective.

- OpenShift User Workload Monitoring (UWM) is enabled in the cluster for Prometheus metrics collection. For instructions on how to do this, refer to, [the official Red Hat docs](https://docs.redhat.com/en/documentation/monitoring_stack_for_red_hat_openshift/4.20/html/configuring_user_workload_monitoring/index).

- You have installed the `custom-metrics-autoscaler` operator from OperatorHub. For more information on how to do this, refer to the [Red Hat official documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/nodes/automatically-scaling-pods-with-the-custom-metrics-autoscaler-operator) on installing it.

- You have installed the `Red Hat Connectivity Link` operator from OperatorHub. For more information on how to do this, refer to the [Red Hat official documentation](https://docs.redhat.com/en/documentation/red_hat_connectivity_link/1.3/html/installing_on_openshift_container_platform/index) on installing it.

- You have the `Red Hat OpenShift Service Mesh 3` operator installed in your cluster. You should have this by default on any Openshift cluster version `4.20` or later.

- You have installed {productname-long} {vernum}.

- A `DataScienceClusterInitialization` (DSCI) and `DataScienceCluster` (DSC) exist in your cluster, enabling the `workload-variant-autoscaler-controller-manager`, `llmisvc-controller-manager` and `kserve-controller-manager`. The `DataScienceClusterInitialization` gets created by the Red Hat Openshift-AI operator out of the box for you. This is an sample exerpt from the `DataScienceCluster` manifest that enables the WVA controller:

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

```bash
```

### Granting KEDA permissions to read from Openshift Monitoring

The Custom Metrics Autoscaler Operator does not automatically have permission to read metrics from the openshift-monitoring or openshift-user-workload-monitoring stack because OpenShift restricts access to cluster monitoring APIs. Access to Prometheus and Thanos endpoints requires explicit RBAC permissions such as the cluster-monitoring-view role. There are two steps to this, **authorization** and **authentication**, lets start with the former. 

#### Authorizing KEDA to OCP UWM metrics

To grant KEDA authorizaiton to see view metrics we will need a `ServiceAccount`, `Secret` to back the service account as a token and `ClusterRoleBinding` granting `cluster-monitoring-view` to our `ServiceAccount`.

You can do this by applying the following yaml:

```bash
oc apply -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: keda-metrics-reader
  namespace: openshift-keda
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
    namespace: openshift-keda
---
apiVersion: v1
kind: Secret
metadata:
  name: keda-metrics-reader-token
  namespace: openshift-keda
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

#### Allowing KEDA to Authenticate against Openshift user workload monitoring

To give KEDA the proper permissions, we need to create a `ClusterTriggerAuthentication`, which will reference the `keda-metrics-reader-token` `Secret` acting as a Token for the `keda-metrics-reader` `ServiceAccount`, both of which we created in the previous step. To create our `ClusterTriggerAuthentication` do the following:

```bash
oc apply -f - <<'EOF'
apiVersion: keda.sh/v1alpha1
kind: ClusterTriggerAuthentication
metadata:
  name: keda-thanos-token
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
clustertriggerauthentication.keda.sh/keda-thanos-token created
```

Now KEDA will be able pull metrics from the Openshift User Workload Monitoring stack.

#### Patching the `inferenceservice-config` to use Openshift User Workload Monitoring

In the previous steps we gave KEDA the authorization and authentication it needs to query Thanos Querier in OpenShift User Workload Monitoring. But KEDA doesn't know where to find those metrics or which credentials to use — that information comes from the llmisvc controller, which reads its autoscaling configuration from the `inferenceservice-config` ConfigMap.

By default, this ConfigMap has no autoscaling configuration set. We need to patch it to tell the llmisvc controller:

  1. **Where** to query metrics — the Thanos Querier URL
  2. **How** to authenticate — using the `ClusterTriggerAuthentication` resource
     (`keda-thanos-token`) we created in the previous step

To do this, we can run the a `kubectl patch` command, and then we need to cycle our `llmisvc-controller-manager` deployment, so that it can pickup our configuration changes. You can do so with the following:

```bash
oc patch configmap inferenceservice-config -n redhat-ods-applications \
    --type=json -p '[{"op":"replace","path":"/data/autoscaling-wva-controller-config","value":"{\"prometheus\":{\"url\":\"https://thanos-querier.openshift-monit oring.svc.cluster.local:9091\",\"authModes\":\"bearer\",\"triggerAuthName\":\" keda-thanos-token\",\"triggerAuthKind\":\"ClusterTriggerAuthentication\"}}"}]'
oc rollout restart deployment llmisvc-controller-manager -n redhat-ods-applications
```

This should result in the following logs:

```console
configmap/inferenceservice-config patched
deployment.apps/llmisvc-controller-manager restarted
```

Now, when the `LLMISVC` gets created with Autoscaling configurations, the `llmisvc-controller-manager` will create a KEDA `ScaledObject` with these settings embedded so KEDA knows how to connect to Thanos and authenticate its metric queries.

### Creating an Inference Stack with Autoscaling enabled

Now we are ready to start applying all of our resources related to our `LLMInferenceService`. 

#### Creating the Namespace

Becuase **we only allow for one inference stack per namesapce** at this time, we will create the `autoscaling-example` namespace, although this should work for any namespace provided you adjust the manifests accordingly.

```bash
oc create ns autoscaling-example && oc project autoscaling-example
```

`oc` should respond by telling us our namespace was created and we are using that project:

```console
namespace/autoscaling-example created
Now using project "autoscaling-example" on server "https://api.qfpda-by6c4-2cb.oiah.p3.openshiftapps.com:443".
```

#### Creating the Gateway

Next were going to create a gateway that will use TLS using Openshift Service Mesh. As described in the [pre-requisites seciont](./RHAISTRAT-670-installation-procedure.md#prerequisites), this should come pre-installed with Openshift 4.20+. To create our `Gateway`, we will create the following `Gateway` and `ConfigMap` manifests:

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
        service.beta.openshift.io/serving-cert-secret-name: "autoscaling-example-gateway-tls"
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
spec:
  router:
    scheduler: {}
    route: {}
    gateway:
      refs: # Reference the gateway we created
      - name: autoscaling-example-gateway
        namespace: autoscaling-example
  model:
    name: "unsloth/Meta-Llama-3.1-8B"
    uri: "hf://unsloth/Meta-Llama-3.1-8B"
  storageInitializer:
    enabled: false
  labels:
    inference.optimization/acceleratorName: cpu # If using Accelerators, use the Name of your Accelerator
  scaling:
    minReplicas: 1
    maxReplicas: 5
    wva:
      variantCost: "10.0"
      keda:
        pollingInterval: 5
        cooldownPeriod: 30
  template:
    containers:
      - name: main
        image: ghcr.io/llm-d/llm-d-inference-sim:v0.8.0 # If using Accelerators, use the corresponding inference-image
        command: ["/app/llm-d-inference-sim"]
        args:
          - "--port"
          - "8000"
          - "--model"
          - "unsloth/Meta-Llama-3.1-8B"
          - "--mode"
          - "random"
          - "--max-num-seqs"
          - "5"
          - "--time-to-first-token"
          - "100"
          - "--inter-token-latency"
          - "30"
          - "--ssl-certfile"
          - "/var/run/kserve/tls/tls.crt"
          - "--ssl-keyfile"
          - "/var/run/kserve/tls/tls.key"
        resources:
          requests:
            cpu: "200m"
            memory: "2Gi"
            # If using specialized hardware, add that here
          limits:
            cpu: "1"
            memory: "2Gi"
            # If using specialized hardware, add that here
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

1. `.spec.gateway.refs` - This is how we tell the LLMISVC to use our pre-created `Gateway`
2. `.spec.labels.inference.optimization/acceleratorName` - Tells the Workload Variant Autoscaler what type of accelerator this workload uses (e.g., `A100`, `H100`, `cpu`). WVA uses this to group variants by hardware type and look up accelerator-specific performance data when making scaling decisions. There is no fixed list of valid values — use whatever matches your hardware for semantic identification. If omitted, it defaults to `unknown`.
3. `.spec.scaling` - This is where we specify user-facing scaling configurations at a per `LLMISVC` (AKA per inference stack) level. Not to be confused with the configurations from the  `workload-variant-autoscaler-saturation-scaling-config` `ConfigMap`, which represents the default scaling configurations that the `workload-variant-autoscaler` will use to determine at what threshold it decides to scale your inference workloads up or down. For a list of what can be configured here see below (@AIDIN link to the docs outside procedure).
4. `.spec.containers[0].template.image` - Different inference images are used for different accelerators. 
5. `.spec.containers[0].template.args`, specifically `--ssl-certfile` and `--ssl-keyfile` - This example runs entirely over TLS. These cert and keyfiles are needed to enable the inference container(s) to do this
6. `.spec.containers[0].template.resources` (`requests` and `limits`) - This example is done using CPU with a simulator, but if you are using real accelerators, make sure the request what you through the kuberenets device plugin. 
    - **NOTE:** The same thing applies for accelerated networking devices like RoCE, or Infiniband, although autoscaling internode workloads through `LeaderWorkerSet` falls outside support at this time.
7. `.spec.containers[0].template` Probe schemes (`startupProbe`, `readinessProbe`, and `livenessProbe`) - As describe above this example uses TLS end to end, if you do not ensure you checks use the TLS scheme it will fail to hit the endpoints, and thus declare the pods ready or health.

#### (Optional) Custom Inference Image Metric Name Workaround

This is an important callout if you are using opensource inference images like vLLM itself. By default vLLM exposes metrics to the `vllm` namespace, but in Red Hat AI Inference Server (RHAIIS), we re-map these to the `kserve_vllm` namespace. This means the Workload Variant Autoscaler will only be looking for the kserve namespacd instances of these metrics. It is recommended you use official product images, but if you want to swap out for other open source vLLM based inferencing images, you can use the following `PrometheusRule` to copy `vllm` namespaced metrics to `kserve_vllm` for compatability with both image types:

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

If everything goes right, you should recieve confirmation that your `PrometheusRule` has been created:

```console
prometheusrule.monitoring.coreos.com/vllm-metrics-alias created
```

## Verifying the Autoscaling Behaviour

Breaking the verification of Autoscaling down into three separate steps helps to ensure our functionality while providing useful guards for debugging in case something has gone wrong. Lets examine if our system can do the following:

1. Are our Inference Server metrics showing up in Prometheus?
2. Can the WVA emit metrics back to prometheus to be consumed by KEDA?
3. Can we actually observe an autoscaling event (E2E test)?

### Verify our Component metrics show up in Prometheus

We can start by ensuring that our inputs to the WVA - the inference-server metrics - show up in Prometheus.

```bash
TOKEN=$(oc whoami -t)
THANOS=$(oc get route thanos-querier -n openshift-monitoring -o jsonpath='{.spec.host}')
for m in num_requests_running num_requests_waiting kv_cache_usage_perc; do
    curl -sk -G -H "Authorization: Bearer $TOKEN" "https://$THANOS/api/v1/query" \
      --data-urlencode "query=kserve_vllm:${m}{namespace=\"autoscaling-example\"}" | \
      jq '.data.result[0]'
done
```

You should see three separate JSON payloads for each:

```json
{
  "metric": {
    "__name__": "kserve_vllm:num_requests_running",
    "container": "main",
    "endpoint": "8000",
    "instance": "10.129.0.66:8000",
    "job": "autoscaling-example/kserve-llm-isvc-vllm-engine",
    "llm_isvc_component": "workload",
    "llm_isvc_name": "autoscaling-example-llama",
    "llm_isvc_role": "both",
    "model_name": "unsloth/Meta-Llama-3.1-8B",
    "namespace": "autoscaling-example",
    "pod": "autoscaling-example-llama-kserve-649dc95869-ntlwq",
    "prometheus": "openshift-user-workload-monitoring/user-workload"
  },
  "value": [
    1774817937.592,
    "0"
  ]
}
{
  "metric": {
    "__name__": "kserve_vllm:num_requests_waiting",
    "container": "main",
    "endpoint": "8000",
    "instance": "10.129.0.66:8000",
    "job": "autoscaling-example/kserve-llm-isvc-vllm-engine",
    "llm_isvc_component": "workload",
    "llm_isvc_name": "autoscaling-example-llama",
    "llm_isvc_role": "both",
    "model_name": "unsloth/Meta-Llama-3.1-8B",
    "namespace": "autoscaling-example",
    "pod": "autoscaling-example-llama-kserve-649dc95869-ntlwq",
    "prometheus": "openshift-user-workload-monitoring/user-workload"
  },
  "value": [
    1774817937.887,
    "0"
  ]
}
{
  "metric": {
    "__name__": "kserve_vllm:kv_cache_usage_perc",
    "container": "main",
    "endpoint": "8000",
    "instance": "10.129.0.66:8000",
    "job": "autoscaling-example/kserve-llm-isvc-vllm-engine",
    "llm_isvc_component": "workload",
    "llm_isvc_name": "autoscaling-example-llama",
    "llm_isvc_role": "both",
    "model_name": "unsloth/Meta-Llama-3.1-8B",
    "namespace": "autoscaling-example",
    "pod": "autoscaling-example-llama-kserve-649dc95869-ntlwq",
    "prometheus": "openshift-user-workload-monitoring/user-workload"
  },
  "value": [
    1774817938.180,
    "0"
  ]
}
```

**NOTE:** Currently the WVA does not currently use metrics from the scheduler but this is coming soon.


### Verify WVA is emitting metrics back to Prometheus

The three metrics were looking for are `wva_current_replicas`, `wva_desired_replicas`, and `wva_desired_ratio`. We can grab those with the following

```bash
TOKEN=$(oc whoami -t)
THANOS=$(oc get route thanos-querier -n openshift-monitoring -o jsonpath='{.spec.host}')
for m in wva_desired_replicas wva_current_replicas wva_desired_ratio wva_replica_scaling_total; do
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
    "accelerator_type": "cpu",
    "endpoint": "https",
    "exported_namespace": "autoscaling-example",
    "instance": "10.128.0.64:8443",
    "job": "workload-variant-autoscaler-controller-manager-metrics-service",
    "namespace": "redhat-ods-applications",
    "pod": "workload-variant-autoscaler-controller-manager-85b8d895cd-x6gnr",
    "prometheus": "openshift-user-workload-monitoring/user-workload",
    "service": "workload-variant-autoscaler-controller-manager-metrics-service",
    "variant_name": "autoscaling-example-llama-kserve-va"
  },
  "value": [
    1774819127.020,
    "1"
  ]
}
{
  "metric": {
    "__name__": "wva_current_replicas",
    "accelerator_type": "cpu",
    "endpoint": "https",
    "exported_namespace": "autoscaling-example",
    "instance": "10.128.0.64:8443",
    "job": "workload-variant-autoscaler-controller-manager-metrics-service",
    "namespace": "redhat-ods-applications",
    "pod": "workload-variant-autoscaler-controller-manager-85b8d895cd-x6gnr",
    "prometheus": "openshift-user-workload-monitoring/user-workload",
    "service": "workload-variant-autoscaler-controller-manager-metrics-service",
    "variant_name": "autoscaling-example-llama-kserve-va"
  },
  "value": [
    1774819127.309,
    "1"
  ]
}
{
  "metric": {
    "__name__": "wva_desired_ratio",
    "accelerator_type": "cpu",
    "endpoint": "https",
    "exported_namespace": "autoscaling-example",
    "instance": "10.128.0.64:8443",
    "job": "workload-variant-autoscaler-controller-manager-metrics-service",
    "namespace": "redhat-ods-applications",
    "pod": "workload-variant-autoscaler-controller-manager-85b8d895cd-x6gnr",
    "prometheus": "openshift-user-workload-monitoring/user-workload",
    "service": "workload-variant-autoscaler-controller-manager-metrics-service",
    "variant_name": "autoscaling-example-llama-kserve-va"
  },
  "value": [
    1774819127.612,
    "1"
  ]
}
```

### Verifying the inference worker gets Autoscaled

This is the fun part; we get to watch our inference workers scale before our very eyes! This script will do the following:
  - Check if your OCP environment has LoadBalancer integration, and if not open up a port-forward to the gateway as a backgrounded subshell
  - Send sustained load of inference requests through the gateway as a backgrounded subshell
  - watch for scaling events by checking replicas of our `autoscaling-example-llama-kserve` `Deployment` every 5 seconds

```bash
case $(oc get infrastructure cluster -o jsonpath='{.status.platformStatus.type}') in
    AWS|GCP|Azure|IBMCloud) LOAD_BALANCER_INTEGRATION=true ;;
    *) LOAD_BALANCER_INTEGRATION=false ;;
esac
if [ "${LOAD_BALANCER_INTEGRATION}" = "false" ]; then
    oc port-forward svc/autoscaling-example-gateway-data-science-gateway-class -n autoscaling-example 9443:443 &>/dev/null &
    PF_PID=$!
    sleep 2
    URL=https://localhost:9443/v1/chat/completions
else
    URL=$(oc get llminferenceservice autoscaling-example-llama -n autoscaling-example -o jsonpath='{.status.url}')/v1/chat/completions
fi

BODY='{"model":"unsloth/Meta-Llama-3.1-8B","messages":[{"role":"user","content":"Tell me a long story"}],"max_tokens":100}'

# Send sustained load in a subshell, capture its PID for cleanup
(while true; do for i in $(seq 1 20); do curl -sk "$URL" -H "Content-Type: application/json" -d "$BODY" -o /dev/null & done; wait; done) &>/dev/null &
LOAD_PID=$!
trap 'kill -- -$LOAD_PID 2>/dev/null; kill $PF_PID 2>/dev/null; wait 2>/dev/null; echo "Stopped."' EXIT INT TERM

# Watch replicas scale (Ctrl+C triggers cleanup)
watch -n5 'oc get deployment autoscaling-example-llama-kserve -n autoscaling-example -o jsonpath="Replicas: {.spec.replicas} ({.status.readyReplicas} ready)"'
```

We should be able to see it going from our orrigional 1 replica:

```console
Replicas: 1 (1 ready)
```

To the scaling event happening where it starts spinning up another inference server replica:

```console
Replicas: 2 (1 ready)
```

To when the second pod comes online:

```console
Replicas: 2 (2 ready)
```

And finally, some time after load ceases and falls before the algorithm's detected threshold, it will scale back down to 1 replica:

```console
Replicas: 1 (1 ready)
```
