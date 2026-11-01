#!/usr/bin/env bash
# Copy the GatewayConfig CRD from opendatahub-operator into this chart.
#
# Source (operator):
#   config/crd/bases/services.platform.opendatahub.io_gatewayconfigs.yaml
# Destination (this chart):
#   files/gatewayconfig-crd.yaml
#
# The chart renders this file as a regular Helm resource. GatewayConfig is applied
# by a post-install/post-upgrade hook after the CRD becomes Established.
#
# Usage:
#   ./sync-gatewayconfig-crd.sh /path/to/opendatahub-operator
#   OPERATOR_DIR=../opendatahub-operator ./sync-gatewayconfig-crd.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

OPERATOR_DIR="${1:-${OPERATOR_DIR:-}}"
if [[ -z "${OPERATOR_DIR}" ]]; then
	echo "Usage: $0 /path/to/opendatahub-operator" >&2
	echo "   or: OPERATOR_DIR=/path/to/opendatahub-operator $0" >&2
	exit 1
fi

SRC="${OPERATOR_DIR}/config/crd/bases/services.platform.opendatahub.io_gatewayconfigs.yaml"
DST="${CHART_DIR}/files/gatewayconfig-crd.yaml"

if [[ ! -f "${SRC}" ]]; then
	echo "ERROR: GatewayConfig CRD not found at ${SRC}" >&2
	echo "In the operator repo run: make manifests" >&2
	exit 1
fi

mkdir -p "${CHART_DIR}/files"

# Drop YAML document separators; the template emits a single CRD object.
sed '/^---[[:space:]]*$/d' "${SRC}" > "${DST}"

echo "Wrote ${DST}"
