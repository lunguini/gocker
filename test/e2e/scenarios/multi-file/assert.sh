#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. Override env wins on the existing 'server' service.
src=$(gocker_exec "$PROJECT" server -- printenv SOURCE 2>/dev/null | tr -d '[:space:]')
if [ "$src" = "override" ]; then
    log_pass "override value wins (SOURCE=override on server)"
else
    log_fail "expected SOURCE=override on server, got '$src'"
    fail_count=$((fail_count + 1))
fi

added=$(gocker_exec "$PROJECT" server -- printenv ADDED_BY_OVERRIDE 2>/dev/null | tr -d '[:space:]')
if [ "$added" = "yes" ]; then
    log_pass "new key from override is present on server"
else
    log_fail "ADDED_BY_OVERRIDE missing on server (got '$added')"
    fail_count=$((fail_count + 1))
fi

# 2. The 'client' service from the override file actually exists and runs.
ps_out=$(compose_cmd -p "$PROJECT" ps 2>&1)
if echo "$ps_out" | grep -qE '\bclient\b'; then
    log_pass "client service from override file is running"
else
    log_fail "client service from override file is NOT in compose ps"
    log_info "ps output was:"
    echo "$ps_out" | head -10 >&2
    fail_count=$((fail_count + 1))
fi

# 3. The client (defined in override file) can reach the server (defined in
#    base file). On the gocker-compose path this resolves the bare service
#    name 'server' through compose's network DNS. On the docker-api path it
#    can't: nerdctl-in-VM has no `--network-alias` on run and no
#    `network connect/disconnect`, so docker compose's per-service DNS
#    aliases never get applied. Containers ARE on the same network and
#    reachable by their full names — fall back to that under docker-api so
#    we still verify cross-file wiring, and surface the DNS gap as a
#    targeted soft-fail.
target="server"
if [ "${E2E_MODE:-gocker}" = "docker-api" ]; then
    target="${PROJECT}-server-1"
fi
if RETRY_MATCH='Welcome to nginx' \
    retry_exec_capture 30 "$PROJECT" client -- wget -q -O- --timeout=2 "http://$target/" \
    >/dev/null 2>&1; then
    log_pass "client reached server via $target"
else
    log_fail "client could not reach $target:80 — cross-file networking broken"
    fail_count=$((fail_count + 1))
fi

# Service-name DNS — gocker-compose has it, docker-api doesn't.
if [ "${E2E_MODE:-gocker}" = "docker-api" ]; then
    if RETRY_MATCH='Welcome to nginx' \
        retry_exec_capture 5 "$PROJECT" client -- wget -q -O- --timeout=2 http://server/ \
        >/dev/null 2>&1; then
        log_pass "service-name DNS works in docker-api (regression of nerdctl limitation)"
    else
        log_warn "service-name DNS not resolved in docker-api mode (known nerdctl limitation: no embedded DNS / --network-alias)"
    fi
fi

exit "$fail_count"
