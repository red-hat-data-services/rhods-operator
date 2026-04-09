kubectl patch configmap inferenceservice-config -n redhat-ods-applications \
    --type='json' \
    --patch-file=./inferenceservice-config-patch.json
# restart llmisvc controller to force reload of configmap
oc rollout restart deployment llmisvc-controller-manager -n redhat-ods-applications
