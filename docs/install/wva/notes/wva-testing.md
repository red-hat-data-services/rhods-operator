0. Deps (uwm, and RHCL + Metrics autoscaler THROUGH OLM)

1. Deploy ODH: `IMG=quay.io/rhoai/odh-rhel9-operator:rhoai-3.4-ea.2 OPERATOR_NAMESPACE=redhat-ods-operator make deploy`
- 1 error when using the `redhat-ods-operator` ns:
```log
clusterrolebinding.rbac.authorization.k8s.io/opendatahub-operator-controller-manager-rolebinding created
mutatingwebhookconfiguration.admissionregistration.k8s.io/opendatahub-operator-mutating-webhook-configuration created
validatingwebhookconfiguration.admissionregistration.k8s.io/opendatahub-operator-validating-webhook-configuration created
the namespace from the provided object "redhat-ods-operator" does not match the namespace "redhat-ods-operator". You must pass '--namespace=redhat-ods-operator' to perform this operation.
the namespace from the provided object "redhat-ods-operator" does not match the namespace "redhat-ods-operator". You must pass '--namespace=redhat-ods-operator' to perform this operation.
the namespace from the provided object "redhat-ods-operator" does not match the namespace "redhat-ods-operator". You must pass '--namespace=redhat-ods-operator' to perform this operation.
the namespace from the provided object "redhat-ods-operator" does not match the namespace "redhat-ods-operator". You must pass '--namespace=redhat-ods-operator' to perform this operation.
```
- undeploy and go back to `OPERATOR_NAMESPACE=redhat-ods-operator`:
```bash
IMG=quay.io/rhoai/odh-rhel9-operator:rhoai-3.4-ea.2 OPERATOR_NAMESPACE=redhat-ods-operator make deploy
```
    - Pods roll out, but they fail to pull the image

- Then you have to create my pullsecret in the cluster and patch the sa to use it:
```bash
kubectl create secret docker-registry greg-pull-secret \
    --from-file=.dockerconfigjson=$HOME/.config/containers/auth.json
```
- edit the `opendatahub-operator-controller-manager` sa in the `redhat-ods-operator` namespace to use the `greg-pull-secret` I created, and then pods rollout:
```bash
k get po -n redhat-ods-operator
NAME                                                      READY   STATUS    RESTARTS   AGE
opendatahub-operator-controller-manager-dccbfc689-jmr4n   1/1     Running   0          12m
opendatahub-operator-controller-manager-dccbfc689-lmr45   1/1     Running   0          11m
opendatahub-operator-controller-manager-dccbfc689-n4x7f   1/1     Running   0          12m
```

2. Create the DSCinitialization
```bash
oc apply -f ./config/samples/dscinitialization_v2_dscinitialization.yaml
```


3. DSC. I deploy with the followign dsc:
```yaml
apiVersion: datasciencecluster.opendatahub.io/v2
kind: DataScienceCluster
metadata:
  name: default-dsc
  labels:
    app.kubernetes.io/name: datasciencecluster
spec:
  components:
    dashboard:
      managementState: "Removed"
    aipipelines:
      managementState: "Removed"
    kserve:
      managementState: "Managed"
      nim:
        managementState: "Managed"
      rawDeploymentServiceConfig: "Headed"
      wva:
        managementState: "Managed"
    kueue:
      managementState: "Removed"
    trainingoperator:
      managementState: "Removed"
    trainer:
      managementState: "Removed"
    ray:
      managementState: "Removed"
    workbenches:
      managementState: "Managed"
    trustyai:
      managementState: "Removed"
    modelregistry:
      managementState: "Removed"
      registriesNamespace: "odh-model-registries"
    feastoperator:
      managementState: "Removed"
    llamastackoperator:
      managementState: "Removed"
    mlflowoperator:
      managementState: "Removed"
    sparkoperator:
      managementState: "Removed"
```
    After applying this I see: 
```bash
k get dsc -A
NAME          READY   REASON
default-dsc   True
k get pods -n opendatahub
NAME                                                              READY   STATUS    RESTARTS   AGE
kserve-controller-manager-7c668b956-s8rq5                         1/1     Running   0          3m2s
llmisvc-controller-manager-994fd758-jkxww                         1/1     Running   0          3m2s
notebook-controller-deployment-c9cd8cf4b-gkvd6                    1/1     Running   0          3m15s
odh-model-controller-546bc6975d-8649d                             1/1     Running   0          3m17s
odh-notebook-controller-manager-6f9b5fc448-jn4sh                  1/1     Running   0          3m16s
workload-variant-autoscaler-controller-manager-749f8595b6-r2scv   1/1     Running   0          3m15s
k logs pod workload-variant-autoscaler-controller-manager-749f8595b6-r2scv -n opendatahub
k logs pod/workload-variant-autoscaler-controller-manager-749f8595b6-r2scv
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:140	Logger initialized
2026-03-25T21:39:17Z	INFO	config/loader.go:53	Configuration loaded successfully
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:154	Configuration loaded successfully
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:291	Watching single namespace	{"namespace": "opendatahub"}
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:306	Setting up indexes
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:311	Indexes setup completed
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:314	Creating metrics emitter instance
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:317	Metrics emitter created successfully
2026-03-25T21:39:17Z	INFO	config/config.go:421	Updated global saturation config	{"oldEntries": 0, "newEntries": 1}
2026-03-25T21:39:17Z	INFO	setup	controller/configmap_reconciler.go:163	Updated global saturation config from ConfigMap	{"entries": 1}
2026-03-25T21:39:17Z	DEBUG	setup	controller/configmap_bootstrap.go:67	Bootstrap ConfigMap not found, continuing with defaults	{"name": "wva-model-scale-to-zero-config", "namespace": "opendatahub"}
2026-03-25T21:39:17Z	INFO	setup	controller/configmap_bootstrap.go:58	Initial ConfigMap bootstrap completed	{"targets": 2}
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:336	Initial ConfigMap bootstrap completed
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:350	Initializing Prometheus client	{"address": "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091", "tlsEnabled": true}
2026-03-25T21:39:17Z	INFO	utils/utils.go:409	Prometheus API validation successful with query	{"query": "up"}
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:375	Prometheus client and API wrapper initialized and validated successfully
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:500	Starting manager
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:510	Registering custom metrics with Prometheus registry
2026-03-25T21:39:17Z	INFO	controller-runtime.metrics	server/server.go:208	Starting metrics server
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:163	disabling http/2
2026-03-25T21:39:17Z	INFO	manager/server.go:83	starting server	{"name": "health probe", "addr": "[::]:8081"}
2026-03-25T21:39:17Z	LEVEL(-2)	controller-runtime.cache	cache/reflector.go:439	Caches populated	{"type": "*v1alpha1.VariantAutoscaling", "reflector": "pkg/mod/k8s.io/client-go@v0.34.5/tools/cache/reflector.go:290"}
I0325 21:39:17.262337       1 leaderelection.go:257] attempting to acquire leader lease opendatahub/72dd1cf1.llm-d.ai...
I0325 21:39:17.286941       1 leaderelection.go:271] successfully acquired lease opendatahub/72dd1cf1.llm-d.ai
2026-03-25T21:39:17Z	DEBUG	events	recorder/recorder.go:104	workload-variant-autoscaler-controller-manager-749f8595b6-r2scv_a67505ce-e725-4f53-8f12-5cf735ff5bf1 became leader	{"type": "Normal", "object": {"kind":"Lease","namespace":"opendatahub","name":"72dd1cf1.llm-d.ai","uid":"33182639-4fd8-4ffc-b74b-5057e1ad61ab","apiVersion":"coordination.k8s.io/v1","resourceVersion":"35906"}, "reason": "LeaderElection"}
2026-03-25T21:39:17Z	INFO	setup	cmd/main.go:380	Initializing metrics source registry
2026-03-25T21:39:17Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:39:17Z	INFO	controller/controller.go:353	Starting EventSource	{"controller": "variantAutoscaling", "controllerGroup": "llmd.ai", "controllerKind": "VariantAutoscaling", "source": "channel source: 0xc0003a8230"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:353	Starting EventSource	{"controller": "configmap", "controllerGroup": "", "controllerKind": "ConfigMap", "source": "kind source: *v1.ConfigMap"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:353	Starting EventSource	{"controller": "variantAutoscaling", "controllerGroup": "llmd.ai", "controllerKind": "VariantAutoscaling", "source": "kind source: *v1alpha1.VariantAutoscaling"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:353	Starting EventSource	{"controller": "variantAutoscaling", "controllerGroup": "llmd.ai", "controllerKind": "VariantAutoscaling", "source": "kind source: *v1.ServiceMonitor"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:353	Starting EventSource	{"controller": "variantAutoscaling", "controllerGroup": "llmd.ai", "controllerKind": "VariantAutoscaling", "source": "kind source: *v1.Deployment"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:353	Starting EventSource	{"controller": "inferencepool", "controllerGroup": "inference.networking.x-k8s.io", "controllerKind": "InferencePool", "source": "kind source: *v1alpha2.InferencePool"}
2026-03-25T21:39:17Z	LEVEL(-2)	controller-runtime.cache	cache/reflector.go:439	Caches populated	{"type": "*v1.Deployment", "reflector": "pkg/mod/k8s.io/client-go@v0.34.5/tools/cache/reflector.go:290"}
2026-03-25T21:39:17Z	ERROR	controller-runtime.source.Kind	source/kind.go:75	if kind is a CRD, it should be installed before calling Start	{"kind": "InferencePool.inference.networking.x-k8s.io", "error": "failed to get restmapping: no matches for kind \"InferencePool\" in version \"inference.networking.x-k8s.io/v1alpha2\""}
sigs.k8s.io/controller-runtime/pkg/internal/source.(*Kind[...]).Start.func1.1
	/go/pkg/mod/sigs.k8s.io/controller-runtime@v0.22.5/pkg/internal/source/kind.go:75
k8s.io/apimachinery/pkg/util/wait.loopConditionUntilContext.func1
	/go/pkg/mod/k8s.io/apimachinery@v0.34.5/pkg/util/wait/loop.go:53
k8s.io/apimachinery/pkg/util/wait.loopConditionUntilContext
	/go/pkg/mod/k8s.io/apimachinery@v0.34.5/pkg/util/wait/loop.go:54
k8s.io/apimachinery/pkg/util/wait.PollUntilContextCancel
	/go/pkg/mod/k8s.io/apimachinery@v0.34.5/pkg/util/wait/poll.go:33
sigs.k8s.io/controller-runtime/pkg/internal/source.(*Kind[...]).Start.func1
	/go/pkg/mod/sigs.k8s.io/controller-runtime@v0.22.5/pkg/internal/source/kind.go:68
2026-03-25T21:39:17Z	LEVEL(-2)	controller-runtime.cache	cache/reflector.go:439	Caches populated	{"type": "*v1.ServiceMonitor", "reflector": "pkg/mod/k8s.io/client-go@v0.34.5/tools/cache/reflector.go:290"}
2026-03-25T21:39:17Z	LEVEL(-2)	controller-runtime.cache	cache/reflector.go:439	Caches populated	{"type": "*v1.ConfigMap", "reflector": "pkg/mod/k8s.io/client-go@v0.34.5/tools/cache/reflector.go:290"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:286	Starting Controller	{"controller": "configmap", "controllerGroup": "", "controllerKind": "ConfigMap"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:289	Starting workers	{"controller": "configmap", "controllerGroup": "", "controllerKind": "ConfigMap", "worker count": 1}
2026-03-25T21:39:17Z	INFO	controller/controller.go:286	Starting Controller	{"controller": "variantAutoscaling", "controllerGroup": "llmd.ai", "controllerKind": "VariantAutoscaling"}
2026-03-25T21:39:17Z	INFO	controller/controller.go:289	Starting workers	{"controller": "variantAutoscaling", "controllerGroup": "llmd.ai", "controllerKind": "VariantAutoscaling", "worker count": 1}
2026-03-25T21:39:17Z	INFO	controller/configmap_reconciler.go:163	Updated global saturation config from ConfigMap	{"controller": "configmap", "controllerGroup": "", "controllerKind": "ConfigMap", "ConfigMap": {"name":"workload-variant-autoscaler-saturation-scaling-config","namespace":"opendatahub"}, "namespace": "opendatahub", "name": "workload-variant-autoscaler-saturation-scaling-config", "reconcileID": "dbf33489-e204-442e-984c-74d761e4342b", "entries": 1}
2026-03-25T21:39:17Z	DEBUG	controller/configmap_reconciler.go:86	Ignoring unrecognized ConfigMap	{"controller": "configmap", "controllerGroup": "", "controllerKind": "ConfigMap", "ConfigMap": {"name":"workload-variant-autoscaler-wva-variantautoscaling-config","namespace":"opendatahub"}, "namespace": "opendatahub", "name": "workload-variant-autoscaler-wva-variantautoscaling-config", "reconcileID": "7a380046-cfd2-424b-ab59-0699392f4656", "name": "workload-variant-autoscaler-wva-variantautoscaling-config", "namespace": "opendatahub"}
2026-03-25T21:39:19Z	INFO	controller-runtime.metrics	server/server.go:247	Serving metrics server	{"bindAddress": ":8443", "secure": true}
2026-03-25T21:39:27Z	LEVEL(-2)	controller-runtime.cache	cache/reflector.go:439	Caches populated	{"type": "*v1alpha2.InferencePool", "reflector": "pkg/mod/k8s.io/client-go@v0.34.5/tools/cache/reflector.go:290"}
2026-03-25T21:39:27Z	INFO	controller/controller.go:286	Starting Controller	{"controller": "inferencepool", "controllerGroup": "inference.networking.x-k8s.io", "controllerKind": "InferencePool"}
2026-03-25T21:39:27Z	INFO	controller/controller.go:289	Starting workers	{"controller": "inferencepool", "controllerGroup": "inference.networking.x-k8s.io", "controllerKind": "InferencePool", "worker count": 1}
2026-03-25T21:39:47Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:40:17Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:40:47Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:41:17Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:41:47Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:42:17Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:42:47Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
```

4. Edit the saturation scaling config to PROVE that it honors it:
```bash
k edit cm -n opendatahub workload-variant-autoscaler-saturation-scaling-config # I just adjust kvcache scale percentage by 1 basis point
configmap/workload-variant-autoscaler-saturation-scaling-config edited
k logs pod/workload-variant-autoscaler-controller-manager-749f8595b6-r2scv -n opendatahub | tail -n 3
2026-03-25T21:49:04Z	INFO	controller/configmap_reconciler.go:163	Updated global saturation config from ConfigMap	{"controller": "configmap", "controllerGroup": "", "controllerKind": "ConfigMap", "ConfigMap": {"name":"workload-variant-autoscaler-saturation-scaling-config","namespace":"opendatahub"}, "namespace": "opendatahub", "name": "workload-variant-autoscaler-saturation-scaling-config", "reconcileID": "e55bf300-8d25-4ab3-a9c1-11f356e89b15", "entries": 1}
2026-03-25T21:49:17Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
2026-03-25T21:49:47Z	INFO	saturation/engine.go:180	No active VariantAutoscalings found, skipping optimization
```

5. patch `inferenceservice-config` cm in `opendatahub` ns for prometheus URL
```bash
kubectl patch configmap inferenceservice-config -n opendatahub --type merge \
    -p '{"data":{"autoscaling-wva-controller-config":"{\"prometheus\":{\"url\":\"https://thanos-querier.openshift-monitoring.svc.cluster.local:9092\"}}"}}'
```

6. create llmisvc
```bash
k create -ns greg && oc project greg
# apply the following llmisvc:
apiVersion: serving.kserve.io/v1alpha2
kind: LLMInferenceService
metadata:
  name: sim-llama
  namespace: greg
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8000"
    prometheus.io/path: "/metrics"
spec:
  model:
    name: "unsloth/Meta-Llama-3.1-8B"
    uri: "hf://unsloth/Meta-Llama-3.1-8B"
  storageInitializer:
    enabled: false
  labels:
    inference.optimization/acceleratorName: cpu
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
        image: ghcr.io/llm-d/llm-d-inference-sim:v0.5.1
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
        resources:
          requests:
            cpu: "200m"
            memory: "2Gi"
          limits:
            cpu: "1"
            memory: "2Gi"
        startupProbe:
        httpGet:
          path: /health
          port: 8000
          scheme: HTTP
        failureThreshold: 60
        periodSeconds: 10
      readinessProbe:
        httpGet:
          path: /health
          port: 8000
          scheme: HTTP
      livenessProbe:
        httpGet:
          path: /health
          port: 8000
          scheme: HTTP

k apply -f config/samples/custom/sim_llmisvc.yaml
llminferenceservice.serving.kserve.io/sim-llama created
```
- Now I hit a bug where my RHCL wasn't installed properlly so it was missing kuadrant CRDs. LLMISVC controller does not continually re-check for them so its just failed. I had to delete the llmisvc controller to get it to come back up

- at this point i get llmisvc ready:
```
k get llmisvc
NAME        URL   READY   REASON   AGE
sim-llama         True             10m
```

- but my scaledObject is not ready:
```
 k get scaledobject
NAME                    SCALETARGETKIND   SCALETARGETNAME    MIN   MAX   READY   ACTIVE   FALLBACK   PAUSED   TRIGGERS   AUTHENTICATIONS   AGE
sim-llama-kserve-keda                     sim-llama-kserve   1     5                                                                       11m
```

- Missing the kedacontroller instance:
```yaml
apiVersion: keda.sh/v1alpha1
kind: KedaController
metadata:
    name: keda
    namespace: openshift-keda
spec:
    watchNamespace: ""
```
