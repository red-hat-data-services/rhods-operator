kubectl apply -f 07a-service-account.yaml
kubectl apply -f 07b-cluster-role-binding.yaml
kubectl apply -f 07c-trigger-authentication.yaml
./07d-run-patch.sh
