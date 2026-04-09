

Apply the `inferenceservice-config` patch last:
```
kubectl patch configmap inferenceservice-config -n opendatahub \
    --type='json' \
    --patch-file=./10-inferenceservice-config-patch.json
# restart llmisvc controller to force reload of configmap
oc rollout restart deployment llmisvc-controller-manager -n opendatahub
```
