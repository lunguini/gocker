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

# Read per-scenario extra compose args (e.g. multiple -f files).
# Relative paths after -f / --file are resolved against the scenario dir so
# gocker's compose proxy can translate them into VM-internal paths.
COMPOSE_EXTRA=""
if [ -f "$SCENARIO_DIR/compose.args" ]; then
    raw_args=$(cat "$SCENARIO_DIR/compose.args")
    # shellcheck disable=SC2086
    set -- $raw_args
    prev=""
    new_args=""
    for tok in "$@"; do
        case "$prev" in
            -f|--file|--project-directory)
                case "$tok" in
                    /*) ;;
                    *) tok="$SCENARIO_DIR/$tok" ;;
                esac
                ;;
        esac
        if [ -z "$new_args" ]; then
            new_args="$tok"
        else
            new_args="$new_args $tok"
        fi
        prev="$tok"
    done
    COMPOSE_EXTRA="$new_args"
fi
export COMPOSE_EXTRA

# Preflight cleanup in case a prior run crashed mid-flight.
# shellcheck disable=SC2086
(cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA down -v --remove-orphans 2>/dev/null || true)

# Bring up.
log_info "compose up -d"
# shellcheck disable=SC2086
if ! (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA up -d); then
    log_fail "compose up failed for $SCENARIO"
    # shellcheck disable=SC2086
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA logs 2>/dev/null | tail -80) || true
    # shellcheck disable=SC2086
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA down -v --remove-orphans 2>/dev/null) || true
    exit 1
fi

# Run assertions.
assert_rc=0
log_info "running assertions"
if ! (cd "$SCENARIO_DIR" && PROJECT="$PROJECT" GOCKER="$GOCKER" COMPOSE_EXTRA="$COMPOSE_EXTRA" "$SCENARIO_DIR/assert.sh"); then
    assert_rc=1
    log_fail "assertions failed for $SCENARIO"
    # shellcheck disable=SC2086
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA ps) || true
    # shellcheck disable=SC2086
    (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA logs 2>/dev/null | tail -80) || true
fi

# Teardown and verify clean state.
log_info "compose down -v"
# shellcheck disable=SC2086
if ! (cd "$SCENARIO_DIR" && "$GOCKER" compose -p "$PROJECT" $COMPOSE_EXTRA down -v --remove-orphans); then
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
