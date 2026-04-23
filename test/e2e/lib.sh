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

# wait_for_healthy waits up to $2 seconds for every service in the current
# compose project to report healthy. Services without a healthcheck are treated
# as healthy as soon as they're running.
wait_for_healthy() {
    local project="$1"; local timeout="${2:-60}"
    local deadline=$(( $(date +%s) + timeout ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        local ps_out
        ps_out=$("$GOCKER" compose -p "$project" ps --format json 2>/dev/null || true)
        if [ -n "$ps_out" ] && ! echo "$ps_out" | grep -qE '"(starting|unhealthy)"'; then
            return 0
        fi
        sleep 2
    done
    log_fail "services did not become healthy within ${timeout}s"
    "$GOCKER" compose -p "$project" ps || true
    return 1
}

# wait_for_log waits up to $3 seconds for $2 (grep pattern) to appear in the
# logs of service $1 in the current compose project.
wait_for_log() {
    local project="$1"; local service="$2"; local pattern="$3"; local timeout="${4:-60}"
    local deadline=$(( $(date +%s) + timeout ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if "$GOCKER" compose -p "$project" logs "$service" 2>/dev/null | grep -qE "$pattern"; then
            return 0
        fi
        sleep 2
    done
    log_fail "pattern '$pattern' not seen in $service logs within ${timeout}s"
    "$GOCKER" compose -p "$project" logs "$service" | tail -40 || true
    return 1
}

# gocker_exec runs a command inside a compose service via gocker compose exec.
# Usage: gocker_exec PROJECT SERVICE -- cmd args...
gocker_exec() {
    local project="$1"; local service="$2"; shift 2
    "$GOCKER" compose -p "$project" exec -T "$service" "$@"
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
