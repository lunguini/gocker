#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

src=$(gocker_exec "$PROJECT" app -- printenv SOURCE 2>/dev/null | tr -d '[:space:]')
if [ "$src" = "override" ]; then
    log_pass "override value wins (SOURCE=override)"
else
    log_fail "expected SOURCE=override, got '$src'"
    fail_count=$((fail_count + 1))
fi

added=$(gocker_exec "$PROJECT" app -- printenv ADDED_BY_OVERRIDE 2>/dev/null | tr -d '[:space:]')
if [ "$added" = "yes" ]; then
    log_pass "new key from override is present"
else
    log_fail "ADDED_BY_OVERRIDE missing (got '$added')"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
