# How to deploy on clusterbot

1. Make a OCP 4.20 or later cluster on clusterbot 
2. run standup.sh (should just work out of the box)

## Pre-reqs

You need a pull-secret that can pull RHOAI images. Put this in `~/.config/containers/auth.json`.

## There's other stuff in my install docs not the official procedure, why is that?

This is because some of these scripts are related to things customers wont have to deal with. This demo installs the ODH operator from scratch with makefile and the pre-fetched manifests changes that I have in my PR.

### Stack installation

Everything should work with the standup script, but in case anything gets stuck you can go directory by directory.

1. Create UWM configmap, custom-metrics-autoscaler and connectivity link resources (can happen all at once). Wait for those to finish
2. Run the 04-rhods-operator/04-deploy-and-patch-odh-operator-with-sa.sh script. This will deploy the ODH operator with my manifest changes. These changes should be accounted for by the time we go to release, so don't worry that its my custom image and different manfiests. Wait for that to complete.
3. Run the 05-dsc/apply.sh script to create the DSC which will enable WVA, kserve, and llmisvc controllers
4. If testing on clusterbot, you have to run 06-scale-down-non-essential.sh. This will free up space on the cluster so that you can deploy everything else in the demo. Ignore this if you are testing on a real cluster.
5. Run the 07-auth/apply.sh script to create the authentication and authorization for KEDA to see stuff from OpenShift monitoring. Customers will have to do this for DP, documented in the procedure docs
6. Run the 08-llmisvc/apply.sh. Creates the LLMISVC, gateway, and namespace. It will also create recording rules account for the inference-sim image we have in that demo to map `vllm` --> `kserve_vllm` metrics, the latter of which is what WVA will operate on. This is basically a unique workaround for CPU only clusters, any official RHAIIS image will not have to do this
7. Run the verification scripts. We have 3 separate ones:
    - 09-verification/09a-check-inference-metrics.sh - checks if `kserve_vllm:` metrics show up in prometheus
    - 09-verification/09b-check-wva-metrics.sh - checks if the WVA is emitting metrics to prometheus
    - 09-verification/09c-test-autoscaling.sh - this test attempts to produce a scaling event    


