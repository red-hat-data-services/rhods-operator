#!/bin/bash
# Generate self-signed TLS certificate for Kind deployment
# This script is called by kustomize to create TLS secrets at deployment time

set -e

# Directory where certificates will be stored (relative to script location)
SCRIPT_DIR=$(dirname "$(realpath "$0")")
CERT_FILE="$SCRIPT_DIR/tls.crt"
KEY_FILE="$SCRIPT_DIR/tls.key"

# Generate certificate and key if they don't exist or if explicitly requested
if [ "${1:-}" = "generate" ] || [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
    echo "Generating new TLS certificate and key..." >&2

    # Create temporary directory for generation
    TEMP_DIR=$(mktemp -d)
    trap "rm -rf $TEMP_DIR" EXIT

    # Generate self-signed certificate and key
    openssl req -x509 -newkey rsa:2048 -keyout "$TEMP_DIR/tls.key" -out "$TEMP_DIR/tls.crt" \
        -days 365 -nodes -subj "/CN=mlflow.opendatahub.svc.cluster.local" \
        -addext "subjectAltName=DNS:mlflow.opendatahub.svc.cluster.local,DNS:mlflow,DNS:localhost"

    # Move generated files to final location
    mv "$TEMP_DIR/tls.crt" "$CERT_FILE"
    mv "$TEMP_DIR/tls.key" "$KEY_FILE"

    echo "Certificate and key generated successfully" >&2
fi

# Output the certificate or key based on the argument
case "${1:-}" in
    "generate")
        echo "Certificate and key generation completed" >&2
        ;;
    "cert")
        cat "$CERT_FILE"
        ;;
    "key")
        cat "$KEY_FILE"
        ;;
    *)
        echo "Usage: $0 {generate|cert|key}" >&2
        echo "  generate: Generate new certificate and key files" >&2
        echo "  cert:     Output the certificate" >&2
        echo "  key:      Output the private key" >&2
        exit 1
        ;;
esac