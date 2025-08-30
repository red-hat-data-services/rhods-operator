This OpenShift Template generates the Job 'async-upload' developed in: https://github.com/opendatahub-io/model-registry/tree/main/jobs/async-upload

Example generation

```sh
oc process --local -f jobs-async-upload-s3-to-oci-template.yaml -p MODEL_SYNC_MODEL_ID=1 -p MODEL_SYNC_MODEL_VERSION_ID=3 -p MODEL_SYNC_MODEL_ARTIFACT_ID=6 -p MODEL_SYNC_REGISTRY_SERVER_ADDRESS=https://... -p MODEL_SYNC_REGISTRY_PORT=443 -p SOURCE_CONNECTION=my-s3-credentials -p DESTINATION_CONNECTION=my-oci-credentials -p MODEL_SYNC_SOURCE_AWS_KEY=path/in/bucket -o yaml
```
