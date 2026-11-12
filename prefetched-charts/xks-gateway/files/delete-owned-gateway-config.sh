CRD_NAME="gatewayconfigs.services.platform.opendatahub.io"
EXPECTED_OWNER={{ printf "%s/%s" .Release.Namespace .Release.Name | quote }}
if kubectl get "crd/${CRD_NAME}" >/dev/null 2>&1; then
  OWNER=$(kubectl get gatewayconfig/default-gateway --ignore-not-found -o jsonpath='{.metadata.annotations.platform\.opendatahub\.io/gateway-config-owner}')
  if [[ "${OWNER}" == "${EXPECTED_OWNER}" ]]; then
    echo "Deleting default-gateway owned by ${EXPECTED_OWNER}..."
    kubectl delete gatewayconfig/default-gateway --ignore-not-found --timeout=300s
  else
    echo "default-gateway is not owned by ${EXPECTED_OWNER}; leaving it unchanged."
  fi
else
  echo "GatewayConfig CRD is not installed; nothing to delete."
fi
