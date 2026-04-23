#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. All three services from the three files must be in compose ps.
# shellcheck disable=SC2086
ps=$("$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA ps 2>/dev/null || true)
for svc in orchestrator db cache; do
    if echo "$ps" | grep -qE "\\b${svc}\\b"; then
        log_pass "service $svc is running (3-file merge succeeded)"
    else
        log_fail "service $svc missing from compose ps"
        fail_count=$((fail_count + 1))
    fi
done

# 2. Wait for orchestrator to finish its flow (marker file appears in volume).
log_info "waiting for orchestrator to complete its flow..."
if retry_exec 180 "$PROJECT" orchestrator -- test -f /backup/DONE; then
    log_pass "orchestrator finished (DONE marker present)"
else
    log_fail "orchestrator did not finish within 180s"
    fail_count=$((fail_count + 1))
fi

# 3. Answer file in the backup volume contains the value from db.
if RETRY_MATCH='^42$' retry_exec_capture 10 "$PROJECT" orchestrator -- cat /backup/answer.txt >/dev/null; then
    log_pass "backup volume contains the correct value from db (42)"
else
    log_fail "backup volume missing or wrong value"
    fail_count=$((fail_count + 1))
fi

# 4. Cache service has the value (data flow ended in redis).
if RETRY_MATCH='^42$' retry_exec_capture 10 "$PROJECT" orchestrator -- redis-cli -h cache GET answer >/dev/null; then
    log_pass "cache has answer=42 (end-to-end data flow)"
else
    log_fail "cache does not have expected value"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
