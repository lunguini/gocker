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
if compose_cmd -p "$PROJECT" ps 2>/dev/null | grep -qE '\bclient\b'; then
    log_pass "client service from override file is running"
else
    log_fail "client service from override file is NOT in compose ps"
    fail_count=$((fail_count + 1))
fi

# 3. The client (defined in override file) can reach the server (defined in base
#    file) over the compose-network service-name DNS. This is the real
#    multi-file-services interaction test: cross-file service references work.
#    Retry — nginx may take a beat to start serving. retry_exec_capture signature
#    is: TIMEOUT PROJECT SERVICE -- cmd... (see test/e2e/lib.sh).
if RETRY_MATCH='Welcome to nginx' \
    retry_exec_capture 30 "$PROJECT" client -- wget -q -O- --timeout=2 http://server/ \
    >/dev/null 2>&1; then
    log_pass "client reached server via cross-file service-name DNS"
else
    log_fail "client could not reach server:80 — cross-file DNS or service ordering broken"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
