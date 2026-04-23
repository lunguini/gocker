#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

check_env() {
    local key="$1"; local want="$2"
    local got
    got=$(gocker_exec "$PROJECT" app -- printenv "$key" 2>/dev/null | tr -d '[:space:]')
    if [ "$got" = "$want" ]; then
        log_pass "$key=$want"
    else
        log_fail "$key: expected '$want', got '$got'"
        fail_count=$((fail_count + 1))
    fi
}

check_env APP_VERSION 1.2.3
check_env GREETING hello-from-dotenv
check_env FALLBACK default-value

exit "$fail_count"
