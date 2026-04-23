#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

got=$(gocker_exec "$PROJECT" app -- cat /etc/e2e-marker 2>/dev/null | tr -d '[:space:]')
if [ "$got" = "marker=e2e-verified" ]; then
    log_pass "image was built locally and build-arg threaded through (marker=e2e-verified)"
else
    log_fail "marker file missing or wrong (got: '$got') — local build broken?"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
