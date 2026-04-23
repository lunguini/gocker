#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. Broker becomes healthy; probe starts only after (depends_on: service_healthy).
if wait_for_healthy "$PROJECT" 120; then
    log_pass "broker became healthy"
else
    log_fail "broker did not become healthy within 120s"
    fail_count=$((fail_count + 1))
fi

# 2. Probe can resolve 'broker' by service name over the compose network.
if gocker_exec "$PROJECT" probe -- rpk -X brokers=broker:9092 cluster info >/dev/null 2>&1; then
    log_pass "probe reached broker via service-name DNS"
else
    log_fail "probe could not reach broker:9092 — service-name DNS broken?"
    fail_count=$((fail_count + 1))
fi

# 3. Create a topic from probe, verify visible from broker.
gocker_exec "$PROJECT" probe -- rpk -X brokers=broker:9092 topic create e2e-topic >/dev/null 2>&1
if gocker_exec "$PROJECT" broker -- rpk topic list 2>/dev/null | grep -q '^e2e-topic'; then
    log_pass "topic created from probe visible from broker"
else
    log_fail "topic e2e-topic not visible from broker"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
