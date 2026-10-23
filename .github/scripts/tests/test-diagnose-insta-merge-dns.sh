#!/bin/sh
# Test credential isolation and status propagation without network access.
set -eu
script=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)/diagnose-insta-merge-dns.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

# The mock must expand these variables when executed, not when generated.
# shellcheck disable=SC2016
printf '%s\n' '#!/bin/sh
set -eu
if [ "${1:-}" = --version ]; then
    echo "gh version 2.58.0 (mock)"
    exit 0
fi
[ "${GH_TOKEN+x}" != x ]
[ "${GITHUB_TOKEN+x}" != x ]
[ "${GITHUB_ENTERPRISE_TOKEN+x}" != x ]
[ "${HTTPS_PROXY+x}" != x ]
[ "$HOME" = "$GH_CONFIG_DIR" ]
[ "$GH_HOST" = diagnostic.invalid ]
[ "$GH_ENTERPRISE_TOKEN" = diagnostic-placeholder ]
[ "$GH_DEBUG" = 1 ]
[ "$GODEBUG" = netdns=2 ]
[ "$1" = api ]
[ "$2" = https://api.github.com/meta ]
[ "$3" = --jq ]
[ "$4" = .verifiable_password_authentication ]
case "$0" in
    *dns-gh)
        echo "error connecting to api.github.com" >&2
        echo "lookup api.github.com: mock DNS failure" >&2
        exit 1
        ;;
    *success-gh) echo false ;;
esac' > "$work/mock-gh"
chmod +x "$work/mock-gh"
ln -s mock-gh "$work/success-gh"
ln -s mock-gh "$work/dns-gh"

for scenario in success dns; do
    status=0
    TMPDIR="$work" GH_TOKEN=not-a-real-token GITHUB_TOKEN=not-a-real-token \
        GITHUB_ENTERPRISE_TOKEN=not-a-real-token HTTPS_PROXY=not-a-real-proxy \
        sh "$script" cli "$work/$scenario-gh" > "$work/$scenario.log" 2>&1 || status=$?
    case "$scenario" in
        success)
            [ "$status" -eq 0 ]
            grep -q 'CLI request exit status: 0' "$work/$scenario.log"
            ;;
        dns)
            [ "$status" -eq 1 ]
            grep -q 'lookup api.github.com: mock DNS failure' "$work/$scenario.log"
            grep -q 'CLI request exit status: 1' "$work/$scenario.log"
            ;;
    esac
    printf 'PASS: %s probe preserves status and isolates credentials\n' "$scenario"
done
status=0
sh "$script" invalid > "$work/invalid.log" 2>&1 || status=$?
[ "$status" -eq 2 ]
printf 'PASS: invalid mode rejected\n'
