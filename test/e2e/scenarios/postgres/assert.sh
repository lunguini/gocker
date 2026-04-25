#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. Postgres survives ext4 lost+found via the PGDATA subdir injection.
if wait_for_healthy "$PROJECT" 90; then
    log_pass "postgres became healthy (ext4 lost+found workaround worked)"
else
    log_fail "postgres did not become healthy — check PGDATA injection"
    fail_count=$((fail_count + 1))
fi

# 2. SELECT 1 over exec. Poll — nerdctl ignores the healthcheck, so "running"
# doesn't mean ready to accept queries.
if RETRY_MATCH='^1$' retry_exec_capture 90 "$PROJECT" db psql -U e2e -d e2e -Atqc 'SELECT 1;' | grep -q '^1$'; then
    log_pass "SELECT 1 returns 1"
else
    log_fail "SELECT 1 did not return 1"
    fail_count=$((fail_count + 1))
fi

# 3. Create a table, insert data, restart, verify data persists.
# Use retry_exec — the CREATE may race with startup if #2 retried into readiness.
if ! retry_exec 30 "$PROJECT" db psql -U e2e -d e2e -c \
    "CREATE TABLE e2e (v text PRIMARY KEY); INSERT INTO e2e VALUES ('hello');"; then
    log_fail "could not create e2e table"
    fail_count=$((fail_count + 1))
fi

log_info "restarting postgres to verify volume persistence"
compose_cmd -p "$PROJECT" down >/dev/null
(cd "$SCRIPT_DIR" && compose_cmd -p "$PROJECT" up -d) >/dev/null

if ! wait_for_healthy "$PROJECT" 90; then
    log_fail "postgres did not return to healthy after restart"
    fail_count=$((fail_count + 1))
else
    got=$(RETRY_MATCH='^hello$' retry_exec_capture 90 "$PROJECT" db psql -U e2e -d e2e -Atqc 'SELECT v FROM e2e;' | tr -d '[:space:]')
    if [ "$got" = "hello" ]; then
        log_pass "postgres data persisted across restart"
    else
        log_fail "postgres data missing after restart (got: '$got')"
        fail_count=$((fail_count + 1))
    fi
fi

exit "$fail_count"
