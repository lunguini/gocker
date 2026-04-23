#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?PROJECT must be set by the runner}"
: "${GOCKER:?GOCKER must be set by the runner}"

fail_count=0

# 1. Service becomes healthy.
if wait_for_healthy "$PROJECT" 60; then
    log_pass "redis reports healthy"
else
    log_fail "redis never reported healthy"
    fail_count=$((fail_count + 1))
fi

# 2. PING over compose exec.
if gocker_exec "$PROJECT" redis -- redis-cli ping 2>/dev/null | grep -q PONG; then
    log_pass "redis-cli ping returns PONG"
else
    log_fail "redis-cli ping did not return PONG"
    fail_count=$((fail_count + 1))
fi

# 3. Write a key, restart (compose down+up WITHOUT -v), verify key survives.
gocker_exec "$PROJECT" redis -- redis-cli SET e2e_key e2e_value >/dev/null
gocker_exec "$PROJECT" redis -- redis-cli BGREWRITEAOF >/dev/null
# Give the AOF a moment to flush.
sleep 2

log_info "restarting compose (down without -v, then up) to test volume persistence"
"$GOCKER" compose -p "$PROJECT" down >/dev/null
"$GOCKER" compose -p "$PROJECT" up -d >/dev/null

if ! wait_for_healthy "$PROJECT" 60; then
    log_fail "redis did not come back up after restart"
    fail_count=$((fail_count + 1))
else
    got=$(gocker_exec "$PROJECT" redis -- redis-cli GET e2e_key 2>/dev/null)
    if [ "$got" = "e2e_value" ]; then
        log_pass "volume persists data across compose down/up"
    else
        log_fail "key missing after restart (got: '$got')"
        fail_count=$((fail_count + 1))
    fi
fi

exit "$fail_count"
