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

log_section "Scenario: $SCENARIO (project: $PROJECT, mode: $E2E_MODE)"

# Respect a per-scenario opt-out. If a scenario needs gocker-specific
# behavior not reachable via the Docker API path (e.g. gocker compose exec
# flags that compose v2 doesn't expose), drop a file named skip-docker-api
# into the scenario dir with a one-line reason.
if [ "$E2E_MODE" = "docker-api" ] && [ -f "$SCENARIO_DIR/skip-docker-api" ]; then
    log_warn "skip: $(cat "$SCENARIO_DIR/skip-docker-api")"
    exit 0
fi

e2e_setup_mode
trap e2e_teardown_mode EXIT

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
(cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" down -v --remove-orphans 2>/dev/null || true)

# Compose v2 refuses to adopt networks that lack its own labels — which
# gocker's NetworkCreate doesn't yet forward. A leftover ${PROJECT}_default
# from a previous run (gocker mode or crashed docker-api mode) therefore
# fails the next up with "network was found but has incorrect label".
# Always force-remove it as part of preflight.
"$GOCKER" network rm "${PROJECT}_default" 2>/dev/null || true

# Bring up.
log_info "compose up -d"
if ! (cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" up -d); then
    log_fail "compose up failed for $SCENARIO"
    (cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" logs 2>/dev/null | tail -80) || true
    (cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" down -v --remove-orphans 2>/dev/null) || true
    exit 1
fi

# Run assertions.
assert_rc=0
log_info "running assertions"
if ! (cd "$SCENARIO_DIR" && PROJECT="$PROJECT" GOCKER="$GOCKER" COMPOSE_EXTRA="$COMPOSE_EXTRA" E2E_MODE="$E2E_MODE" "$SCENARIO_DIR/assert.sh"); then
    assert_rc=1
    log_fail "assertions failed for $SCENARIO"
    (cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" ps) || true
    (cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" logs 2>/dev/null | tail -80) || true
fi

# Teardown and verify clean state.
log_info "compose down -v"
if ! (cd "$SCENARIO_DIR" && compose_cmd -p "$PROJECT" down -v --remove-orphans); then
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
