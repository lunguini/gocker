#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. Broker becomes healthy; probe starts only after (depends_on: service_healthy).
# NB: nerdctl compose ignores `healthcheck:` and `depends_on:`, so we have to
# probe real readiness below via retry_exec.
if wait_for_healthy "$PROJECT" 120; then
    log_pass "broker became healthy"
else
    log_fail "broker did not become healthy within 120s"
    fail_count=$((fail_count + 1))
fi

# 2. Probe can resolve 'broker' by service name over the compose network.
# Poll — the broker may still be starting up Kafka even though the container is
# running, and `depends_on: service_healthy` is ignored.
if retry_exec 120 "$PROJECT" probe rpk -X brokers=broker:9092 cluster info; then
    log_pass "probe reached broker via service-name DNS"
else
    log_fail "probe could not reach broker:9092 — service-name DNS broken?"
    fail_count=$((fail_count + 1))
fi

# 3. Create a topic from probe, verify visible from broker.
retry_exec 30 "$PROJECT" probe rpk -X brokers=broker:9092 topic create e2e-topic || true
if retry_exec 30 "$PROJECT" broker sh -c "rpk topic list | grep -q '^e2e-topic'"; then
    log_pass "topic created from probe visible from broker"
else
    log_fail "topic e2e-topic not visible from broker"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
