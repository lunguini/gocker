#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. Postgres (with pgvector) is ready.
if retry_exec 120 "$PROJECT" db -- pg_isready -U postgres; then
    log_pass "postgres (pgvector) ready"
else
    log_fail "postgres never became ready"
    fail_count=$((fail_count + 1))
fi

# 2. Redis responds to PING.
if RETRY_MATCH='PONG' retry_exec_capture 60 "$PROJECT" redis -- redis-cli PING >/dev/null; then
    log_pass "redis responds"
else
    log_fail "redis did not respond"
    fail_count=$((fail_count + 1))
fi

# 3. Immich server boots AND successfully connects to both db and redis.
#    The server logs distinctive startup messages when it's done bootstrapping.
#    Typical markers: "Nest application successfully started", "Listening on",
#    or "Immich server is running". Accept any of them — the image has changed
#    its log format across releases.
log_info "waiting for immich server to bootstrap (can take 2-3 minutes cold)..."
deadline=$(( $(date +%s) + 240 ))
started=false
while [ "$(date +%s)" -lt "$deadline" ]; do
    if compose_cmd -p "$PROJECT" logs server 2>/dev/null \
           | grep -Eq 'Nest application successfully started|Listening on|Immich Server is running|Immich server is running|listening on port'; then
        started=true
        break
    fi
    sleep 5
done
if [ "$started" = true ]; then
    log_pass "immich server started (db+redis connectivity OK)"
else
    log_fail "immich server did not print a startup marker within 240s"
    # Dump last 30 log lines so we can see what it did say
    compose_cmd -p "$PROJECT" logs server 2>/dev/null | tail -30
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
