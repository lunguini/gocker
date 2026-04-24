#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# Query the API we're actually testing — a successful 200 + populated body
# covers the full stack of fixes made on 2026-04-24: ImagePull via the
# daemon (container exec -t errno 19), image ref lookup after pull (short
# vs qualified Name), and Cmd containing '-c' not getting reparsed as a
# --cpus flag.
ctrs=$(curl -s --unix-socket "$HOME/.gocker/gocker.sock" 'http://localhost/containers/json?all=1')
for service in server client; do
    name="/${PROJECT}-${service}-1"
    if echo "$ctrs" | grep -qF "\"$name\""; then
        log_pass "$service container present via /containers/json"
    else
        log_fail "$service container $name not in /containers/json"
        fail_count=$((fail_count + 1))
    fi
done

# Qualified image ref must resolve post-pull — proves the nerdctl parser
# preserves the canonical Name and that imageRefMatches expands short to
# qualified.
img_status=$(curl -s -o /dev/null -w '%{http_code}' --unix-socket "$HOME/.gocker/gocker.sock" \
    "http://localhost/images/docker.io%2Flibrary%2Falpine%3A3/json")
if [ "$img_status" = "200" ]; then
    log_pass "qualified image ref resolves via /images/{name}/json"
else
    log_fail "qualified image ref returned HTTP $img_status (want 200)"
    fail_count=$((fail_count + 1))
fi

# Sanity: container inspect by name from the API (what lazydocker polls).
inspect_status=$(curl -s -o /dev/null -w '%{http_code}' --unix-socket "$HOME/.gocker/gocker.sock" \
    "http://localhost/containers/${PROJECT}-server-1/json")
if [ "$inspect_status" = "200" ]; then
    log_pass "container inspect by name returns 200"
else
    log_fail "container inspect returned HTTP $inspect_status (want 200)"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
