#!/usr/bin/env bash
set -uo pipefail
# Note: intentionally NOT using 'set -e' — we want to continue past assert
# failures to ensure teardown runs, and return the correct exit code.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

if [ $# -lt 1 ]; then
    echo "usage: $0 <scenario-name>" >&2
    exit 2
fi

SCENARIO="$1"
SCENARIO_DIR="$SCRIPT_DIR/scenarios/$SCENARIO"
PROJECT="gocker-e2e-${SCENARIO}"

if [ ! -d "$SCENARIO_DIR" ]; then
    log_fail "scenario not found: $SCENARIO_DIR"
    exit 2
fi
if [ ! -x "$SCENARIO_DIR/assert.sh" ]; then
    log_fail "assert.sh missing or not executable: $SCENARIO_DIR/assert.sh"
    exit 2
fi

log_section "Scenario: $SCENARIO (project: $PROJECT)"

# Preflight cleanup in case a prior run crashed mid-flight.
(cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" down -v --remove-orphans 2>/dev/null || true)

# Bring up.
log_info "compose up -d"
if ! (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" up -d); then
    log_fail "compose up failed for $SCENARIO"
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" logs 2>/dev/null | tail -80) || true
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" down -v --remove-orphans 2>/dev/null) || true
    exit 1
fi

# Run assertions.
assert_rc=0
log_info "running assertions"
if ! (cd "$SCENARIO_DIR" && PROJECT="$PROJECT" GOCKER="$GOCKER" "$SCENARIO_DIR/assert.sh"); then
    assert_rc=1
    log_fail "assertions failed for $SCENARIO"
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" ps) || true
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" logs 2>/dev/null | tail -80) || true
fi

# Teardown and verify clean state.
log_info "compose down -v"
if ! (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" down -v --remove-orphans); then
    log_fail "compose down failed for $SCENARIO"
    assert_rc=1
fi

if ! assert_clean_state "$PROJECT"; then
    assert_rc=1
fi

if [ "$assert_rc" -eq 0 ]; then
    log_pass "$SCENARIO"
else
    log_fail "$SCENARIO"
fi
exit "$assert_rc"
