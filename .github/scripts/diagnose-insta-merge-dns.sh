#!/bin/sh
# Read-only network probes for RHAI-15286; never invokes a merge or uses real tokens.
# Run on a Linux Docker host: sh .github/scripts/diagnose-insta-merge-dns.sh
set -eu

probe_cli() {
    cli=$1
    config=$(mktemp -d)
    "$cli" --version
    status=0
    # gh requires authentication before running. Give it a placeholder for an
    # unrelated host; the absolute github.com API URL receives no credentials.
    # An empty environment also prevents loading real tokens, proxies or config.
    env -i PATH="$PATH" HOME="$config" GH_CONFIG_DIR="$config" \
        GH_HOST=diagnostic.invalid GH_ENTERPRISE_TOKEN=diagnostic-placeholder \
        GH_NO_UPDATE_NOTIFIER=1 GH_DEBUG=1 GODEBUG=netdns=2 \
        "$cli" api https://api.github.com/meta \
        --jq '.verifiable_password_authentication' || status=$?
    printf 'CLI request exit status: %s\n' "$status"
    return "$status"
}

baseline() {
    uname -a
    head -n 20 /etc/resolv.conf
    status=0
    nslookup api.github.com || status=$?
    printf 'DNS probe exit status: %s\n' "$status"
    status=0
    curl -q -fsS --connect-timeout 15 --max-time 30 -o /dev/null \
        -w 'HTTP %{http_code}; remote %{remote_ip}\n' \
        https://api.github.com/meta || status=$?
    printf 'HTTPS probe exit status: %s\n' "$status"
}

case "${1:-host}" in
    host)
        printf '\n=== Runner host ===\n'
        baseline
        docker version
        # Do not consult any existing registry credentials or Docker config.
        docker_config=$(mktemp -d)
        printf '\n=== Action container (default Docker networking/security) ===\n'
        docker --config "$docker_config" run --rm -i --platform linux/amd64 \
            --entrypoint /bin/sh quay.io/rhoai-konflux/alpine:latest \
            -s -- container < "$0"
        ;;
    container)
        printf '\n=== Container environment ===\n'
        head -n 5 /etc/alpine-release
        # Match the packages in insta-merge's pinned Dockerfile.
        apk add --no-cache bash git curl jq wget tar
        baseline
        failed=0
        for arch in 386 amd64; do
            printf '\n=== GitHub CLI v2.58.0 linux_%s ===\n' "$arch"
            directory=$(mktemp -d)
            archive="gh_2.58.0_linux_${arch}.tar.gz"
            curl -q -fLsS --connect-timeout 15 --max-time 120 \
                "https://github.com/cli/cli/releases/download/v2.58.0/$archive" \
                -o "$directory/gh.tar.gz"
            tar --no-same-owner -xzf "$directory/gh.tar.gz" \
                --strip-components=1 -C "$directory"
            # Preserve failures but still collect the other architecture's result.
            probe_cli "$directory/bin/gh" || failed=1
        done
        exit "$failed"
        ;;
    cli)
        # Allows testing the probe in isolation, without Docker or downloads.
        if [ "$#" -ne 2 ]; then
            echo 'Usage: diagnose-insta-merge-dns.sh cli /path/to/gh' >&2
            exit 2
        fi
        probe_cli "$2"
        ;;
    *)
        echo 'Usage: diagnose-insta-merge-dns.sh [host|container|cli /path/to/gh]' >&2
        exit 2
        ;;
esac
