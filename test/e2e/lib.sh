# test/e2e/lib.sh
# shellcheck shell=bash

# Colors for output.
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()    { printf "${CYAN}[e2e]${NC} %s\n" "$*"; }
log_pass()    { printf "${GREEN}[e2e] ✓ %s${NC}\n" "$*"; }
log_fail()    { printf "${RED}[e2e] ✗ %s${NC}\n" "$*" >&2; }
log_warn()    { printf "${YELLOW}[e2e] ! %s${NC}\n" "$*" >&2; }
log_section() { printf "\n${CYAN}=== %s ===${NC}\n" "$*"; }

# GOCKER is the binary under test. Default to whatever's on PATH; override via env.
: "${GOCKER:=gocker}"

# COMPOSE_EXTRA holds per-scenario extra args (e.g. `-f a.yml -f b.yml`) that
# must be prepended to every compose invocation. Default empty so scenarios
# without a compose.args file behave as before.
: "${COMPOSE_EXTRA:=}"

# E2E_MODE selects which code path drives compose:
#   gocker      — (default) `gocker compose ...` → our compose-proxy,
#                 which shells out to nerdctl inside the shared VM. Tests
#                 the proxy, arg translation, and per-scenario VM state.
#   docker-api  — `docker compose ...` with docker context pointing at
#                 gocker's socket. Tests the Docker-API surface (every
#                 endpoint compose v2 calls) against gocker's daemon.
#                 This path caught all four compose regressions on
#                 2026-04-24 (container-exec -t, image inspect by
#                 qualified name, -c vs --cpus collision, nerdctl parser).
: "${E2E_MODE:=gocker}"

# _DOCKER_CTX_SAVED preserves the user's active docker context so we can
# restore it after the suite. Do not set manually.
_DOCKER_CTX_SAVED=""
_DOCKER_CTX_E2E="gocker-e2e"

e2e_setup_mode() {
    case "$E2E_MODE" in
        gocker) ;;
        docker-api)
            command -v docker >/dev/null || { log_fail "E2E_MODE=docker-api needs the docker CLI on PATH"; exit 2; }
            local sock="$HOME/.gocker/gocker.sock"
            [ -S "$sock" ] || { log_fail "gocker.sock not found at $sock — start the daemon with 'gocker daemon start'"; exit 2; }
            _DOCKER_CTX_SAVED=$(docker context show 2>/dev/null || echo default)
            if ! docker context inspect "$_DOCKER_CTX_E2E" >/dev/null 2>&1; then
                docker context create "$_DOCKER_CTX_E2E" --docker "host=unix://$sock" >/dev/null
            fi
            docker context use "$_DOCKER_CTX_E2E" >/dev/null
            log_info "E2E_MODE=docker-api — docker context: $_DOCKER_CTX_E2E (saved: $_DOCKER_CTX_SAVED)"
            ;;
        *)
            log_fail "unknown E2E_MODE=$E2E_MODE (want: gocker | docker-api)"
            exit 2
            ;;
    esac
}

e2e_teardown_mode() {
    if [ "$E2E_MODE" = "docker-api" ] && [ -n "$_DOCKER_CTX_SAVED" ]; then
        docker context use "$_DOCKER_CTX_SAVED" >/dev/null 2>&1 || true
    fi
}

# compose_cmd runs the compose CLI appropriate for the current E2E_MODE.
# It injects COMPOSE_EXTRA automatically so per-scenario flags (-f a.yml
# -f b.yml, --profile, etc.) don't have to be passed at every call site.
compose_cmd() {
    # shellcheck disable=SC2086
    if [ "$E2E_MODE" = "docker-api" ]; then
        docker compose $COMPOSE_EXTRA "$@"
    else
        "$GOCKER" compose $COMPOSE_EXTRA "$@"
    fi
}

# wait_for_healthy waits up to $2 seconds for every service in the current
# compose project to reach a good state. Requires at least one container to
# exist and that no container is in a bad state (starting, unhealthy, created,
# exited, restarting, dead). Success requires seeing state:"running" (or
# health:"healthy") on at least one service.
#
# IMPORTANT: nerdctl compose on gocker's shared VM currently IGNORES user-defined
# `healthcheck:` blocks, so the ps output always has Health:"". This function can
# only confirm the container is running — NOT that the service inside is ready to
# accept connections. Callers that need real readiness (postgres accepting
# queries, a broker accepting Kafka connections) must use `retry_exec` on top of
# this — see postgres/redpanda scenarios for examples.
wait_for_healthy() {
    local project="$1"; local timeout="${2:-60}"
    local deadline=$(( $(date +%s) + timeout ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        local ps_out
        ps_out=$(compose_cmd -p "$project" ps --format json 2>/dev/null || true)
        # No containers yet — keep waiting.
        if [ -z "$ps_out" ] || [ "$ps_out" = "[]" ] || [ "$ps_out" = "null" ]; then
            sleep 1
            continue
        fi
        # Any bad state means not ready.
        if echo "$ps_out" | grep -qE '"(starting|unhealthy|created|exited|restarting|dead|paused)"'; then
            sleep 1
            continue
        fi
        # Need to see at least one healthy/running status to call it healthy.
        if echo "$ps_out" | grep -qE '"(healthy|running)"'; then
            return 0
        fi
        sleep 1
    done
    log_fail "services did not become healthy within ${timeout}s"
    compose_cmd -p "$project" ps || true
    return 1
}

# retry_exec retries a `gocker compose exec` invocation until it succeeds or the
# timeout elapses. Use this for readiness probes when `healthcheck:` is not
# honored (nerdctl compose on gocker ignores it, so we have to poll from the
# outside).
# Usage: retry_exec TIMEOUT PROJECT SERVICE -- cmd args...
retry_exec() {
    local timeout="$1"; local project="$2"; local service="$3"; shift 3
    if [ "${1:-}" = "--" ]; then shift; fi
    local deadline=$(( $(date +%s) + timeout ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if compose_cmd -p "$project" exec -T "$service" "$@" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done
    return 1
}

# retry_exec_capture is like retry_exec but captures stdout on success and echoes
# it. Returns 0 if the command ever succeeded AND an optional match pattern
# ($1 when set via RETRY_MATCH env) is present in the output.
# Usage: RETRY_MATCH='^1$' retry_exec_capture TIMEOUT PROJECT SERVICE -- cmd args...
retry_exec_capture() {
    local timeout="$1"; local project="$2"; local service="$3"; shift 3
    if [ "${1:-}" = "--" ]; then shift; fi
    local deadline=$(( $(date +%s) + timeout ))
    local out rc
    while [ "$(date +%s)" -lt "$deadline" ]; do
        out=$(compose_cmd -p "$project" exec -T "$service" "$@" 2>/dev/null)
        rc=$?
        if [ "$rc" -eq 0 ]; then
            if [ -z "${RETRY_MATCH:-}" ] || echo "$out" | grep -qE "$RETRY_MATCH"; then
                printf '%s' "$out"
                return 0
            fi
        fi
        sleep 1
    done
    return 1
}

# wait_for_log waits up to $3 seconds for $2 (grep pattern) to appear in the
# logs of service $1 in the current compose project.
wait_for_log() {
    local project="$1"; local service="$2"; local pattern="$3"; local timeout="${4:-60}"
    local deadline=$(( $(date +%s) + timeout ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if compose_cmd -p "$project" logs "$service" 2>/dev/null | grep -qE "$pattern"; then
            return 0
        fi
        sleep 2
    done
    log_fail "pattern '$pattern' not seen in $service logs within ${timeout}s"
    compose_cmd -p "$project" logs "$service" | tail -40 || true
    return 1
}

# gocker_exec runs a command inside a compose service via compose exec.
# Kept as the legacy name for scenarios; prefer compose_cmd directly.
# Usage: gocker_exec PROJECT SERVICE -- cmd args...
gocker_exec() {
    local project="$1"; local service="$2"; shift 2
    compose_cmd -p "$project" exec -T "$service" "$@"
}

# assert_clean_state verifies no containers with the project prefix remain
# after compose down.
assert_clean_state() {
    local project="$1"
    local leftover
    leftover=$("$GOCKER" ps -a --format '{{.Names}}' 2>/dev/null | grep -c "^${project}[-_]" || true)
    if [ "$leftover" -gt 0 ]; then
        log_fail "$leftover leftover container(s) after down"
        return 1
    fi
    return 0
}
