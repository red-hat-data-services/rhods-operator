#!/usr/bin/env bash
# Run benchmark and/or eval jobs against an existing LLMInferenceService.
#
# Usage:
#   ./run.sh [flags] <llmisvc-name> <namespace>
#
# Flags:
#   --bench      Run the guidellm benchmark only
#   --eval       Run the lm-eval evaluation only
#   --url URL    Override the service URL (default: from llmisvc status)
#
# If neither --bench nor --eval is specified, both are run.
# Existing jobs with the same name are deleted before creating new ones.
#
# Examples:
#   ./run.sh llmisvc-singlenode-pd llmd-singlenode-prefill-decode
#   ./run.sh --bench llmisvc-singlenode-pd llmd-singlenode-prefill-decode
#   ./run.sh --eval llmisvc-singlenode-pd llmd-singlenode-prefill-decode

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../../.." && pwd)"

usage() {
  sed -n '2,/^$/s/^# \?//p' "${BASH_SOURCE[0]}"
  exit 1
}

RUN_BENCH=false
RUN_EVAL=false
OVERRIDE_URL=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bench)  RUN_BENCH=true; shift ;;
    --eval)   RUN_EVAL=true; shift ;;
    --url)    OVERRIDE_URL="$2"; shift 2 ;;
    --help|-h) usage ;;
    -*) echo "Unknown option: $1" >&2; usage ;;
    *)  break ;;
  esac
done

NAME="${1:-}"
NAMESPACE="${2:-}"
[[ -z "${NAME}" || -z "${NAMESPACE}" ]] && usage

if [[ "${RUN_BENCH}" == "false" && "${RUN_EVAL}" == "false" ]]; then
  RUN_BENCH=true
  RUN_EVAL=true
fi

MODEL=$(kubectl get llmisvc "${NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.model.name}')
if [[ -z "${MODEL}" ]]; then
  echo "Error: could not determine model name from llmisvc '${NAME}' in namespace '${NAMESPACE}'" >&2
  exit 1
fi

if [[ -n "${OVERRIDE_URL}" ]]; then
  URL="${OVERRIDE_URL}"
else
  URL=$(kubectl get llmisvc "${NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.url}')
  if [[ -z "${URL}" ]]; then
    echo "Error: llmisvc '${NAME}' in namespace '${NAMESPACE}' has no URL (not ready?)" >&2
    exit 1
  fi
fi

echo "LLMInferenceService: ${NAME}"
echo "Namespace:           ${NAMESPACE}"
echo "Model:               ${MODEL}"
echo "URL:                 ${URL}"

run_job() {
  local job_type="$1"
  local base_dir="$2"
  local base_job_name="$3"
  local job_name="${job_type}-${NAME}"

  kubectl delete job "${job_name}" -n "${NAMESPACE}" --ignore-not-found

  local tmpdir
  tmpdir=$(mktemp -d)

  ln -s "${base_dir}" "${tmpdir}/base"
  cat > "${tmpdir}/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: ${NAMESPACE}

resources:
  - base

patches:
  - target:
      kind: Job
      name: ${base_job_name}
    patch: |-
      - op: replace
        path: /metadata/name
        value: "${job_name}"
      - op: replace
        path: /spec/template/spec/containers/0/env/0/value
        value: "${MODEL}"
      - op: replace
        path: /spec/template/spec/containers/0/env/1/value
        value: "${URL}"
EOF

  echo ""
  echo "=== Applying ${job_type} job '${job_name}' ==="
  local apply_rc=0
  kustomize build "${tmpdir}" | kubectl apply -f - || apply_rc=$?
  rm -rf "${tmpdir}"
  if [ "${apply_rc}" -ne 0 ]; then
    echo "Failed to apply ${job_type} job" >&2
    return 1
  fi

  echo "Waiting for pod to start..."
  while ! kubectl get pod -l "job-name=${job_name}" -n "${NAMESPACE}" -o jsonpath='{.items[0].status.phase}' 2>/dev/null | grep -qE 'Running|Succeeded|Failed'; do
    sleep 2
  done
  echo "Streaming logs..."
  kubectl logs -f "job/${job_name}" -n "${NAMESPACE}"

  if kubectl wait --for=condition=complete "job/${job_name}" -n "${NAMESPACE}" --timeout=10s 2>/dev/null; then
    echo "=== ${job_type} job '${job_name}' completed successfully ==="
  else
    echo "=== ${job_type} job '${job_name}' FAILED ===" >&2
    return 1
  fi
}

if [[ "${RUN_BENCH}" == "true" ]]; then
  run_job "bench" "${REPO_ROOT}/test/llmisvc/bench/base" "guidellm"
fi

if [[ "${RUN_EVAL}" == "true" ]]; then
  run_job "eval" "${REPO_ROOT}/test/llmisvc/eval/base" "lm-eval"
fi