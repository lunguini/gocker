#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Gocker smoke test -- exercises every container CLI interaction end-to-end
# ---------------------------------------------------------------------------

GOCKER="${GOCKER_BIN:-gocker}"
# Test image, overridable for environments where anonymous Docker Hub pulls
# are rate-limited/blocked but another image is already cached locally.
SMOKE_IMAGE="${GOCKER_SMOKE_IMAGE:-alpine:latest}"
SMOKE_IMAGE_REPO="$(basename "${SMOKE_IMAGE%%:*}")"
SMOKE_IMAGE_PULLED=1
PREFIX="gocker-smoke-$$"
PASS_COUNT=0
FAIL_COUNT=0
COMPOSE_DIR=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

pass() {
    PASS_COUNT=$((PASS_COUNT + 1))
    printf "${GREEN}  ✓ %s${NC}\n" "$1"
}

fail() {
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf "${RED}  ✗ %s${NC}\n" "$1"
}

section() {
    printf "\n${YELLOW}=== %s ===${NC}\n" "$1"
}

run_test() {
    local desc="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        pass "$desc"
    else
        fail "$desc"
    fi
}

assert_contains() {
    local desc="$1"
    local output="$2"
    local expected="$3"
    if echo "$output" | grep -q "$expected"; then
        pass "$desc"
    else
        fail "$desc (expected '$expected' in output)"
    fi
}

cleanup() {
    printf "\n${YELLOW}=== Cleanup ===${NC}\n"

    # Stop and remove test containers
    for ctr in $("$GOCKER" ps -a 2>/dev/null | grep "$PREFIX" | awk '{print $1}' || true); do
        "$GOCKER" stop "$ctr" 2>/dev/null || true
        "$GOCKER" rm "$ctr" 2>/dev/null || true
    done

    # Remove test networks
    for net in $("$GOCKER" network ls 2>/dev/null | grep "$PREFIX" | awk '{print $1}' || true); do
        "$GOCKER" network rm "$net" 2>/dev/null || true
    done

    # Remove test volumes
    for vol in $("$GOCKER" volume ls 2>/dev/null | grep "$PREFIX" | awk '{print $1}' || true); do
        "$GOCKER" volume rm "$vol" 2>/dev/null || true
    done

    # Compose down if dir exists
    if [[ -n "$COMPOSE_DIR" && -d "$COMPOSE_DIR" ]]; then
        "$GOCKER" compose -f "$COMPOSE_DIR/docker-compose.yml" down 2>/dev/null || true
        rm -rf "$COMPOSE_DIR"
    fi

    echo "Cleanup complete."
}

trap cleanup EXIT

# ---------------------------------------------------------------------------
# 1. Prerequisites
# ---------------------------------------------------------------------------

section "Prerequisites"

if command -v "$GOCKER" >/dev/null 2>&1; then
    pass "gocker binary found at $(command -v "$GOCKER")"
else
    fail "gocker binary not found"
    echo "Cannot continue without gocker. Exiting."
    exit 1
fi

if [[ "$(uname -s)" == "Linux" ]]; then
    if command -v nerdctl >/dev/null 2>&1; then
        pass "nerdctl found at $(command -v nerdctl)"
    else
        fail "nerdctl not found"
        echo "Cannot continue without nerdctl. Exiting."
        exit 1
    fi
elif [[ -x /usr/local/bin/container ]]; then
    pass "container CLI found at /usr/local/bin/container"
else
    fail "container CLI not found at /usr/local/bin/container"
    echo "Cannot continue without container CLI. Exiting."
    exit 1
fi

# ---------------------------------------------------------------------------
# 2. Images
# ---------------------------------------------------------------------------

section "Images"

# Pull, falling back to a locally cached copy when the registry is
# unreachable (anonymous Docker Hub pulls 401/429 from some networks).
if "$GOCKER" pull "$SMOKE_IMAGE" >/dev/null 2>&1; then
    pass "pull $SMOKE_IMAGE"
else
    # Check the local cache before declaring failure. Capture the listing
    # into a variable rather than piping straight into grep -q: with
    # pipefail, grep -q's early exit can SIGPIPE the producer and turn a
    # genuine match into a false negative — and a false negative here means
    # the final rmi deletes an image the machine cannot re-pull. Retry a
    # few times since the listing can be transiently empty after a failed
    # pull.
    SMOKE_IMAGE_CACHED=0
    for _ in 1 2 3; do
        IMAGES_NOW=$("$GOCKER" images 2>/dev/null || true)
        if echo "$IMAGES_NOW" | grep -q "$SMOKE_IMAGE_REPO"; then
            SMOKE_IMAGE_CACHED=1
            break
        fi
        sleep 2
    done
    if [[ "$SMOKE_IMAGE_CACHED" == "1" ]]; then
        SMOKE_IMAGE_PULLED=0
        printf "${YELLOW}  ~ pull failed; using locally cached %s (final rmi will be skipped)${NC}\n" "$SMOKE_IMAGE"
    else
        fail "pull $SMOKE_IMAGE"
    fi
fi

OUTPUT=$("$GOCKER" images 2>/dev/null || true)
assert_contains "$SMOKE_IMAGE_REPO shows in gocker images" "$OUTPUT" "$SMOKE_IMAGE_REPO"

JSON_OUTPUT=$("$GOCKER" images --format json 2>/dev/null || true)
assert_contains "gocker images --format json returns JSON" "$JSON_OUTPUT" "$SMOKE_IMAGE_REPO"

# ---------------------------------------------------------------------------
# 3. Container lifecycle
# ---------------------------------------------------------------------------

section "Container lifecycle"

CNAME="${PREFIX}-lifecycle"

run_test "run detached $SMOKE_IMAGE_REPO (sleep 300)" "$GOCKER" run -d --name "$CNAME" "$SMOKE_IMAGE" sleep 300

PS_OUTPUT=$("$GOCKER" ps 2>/dev/null || true)
assert_contains "container shows in gocker ps" "$PS_OUTPUT" "$CNAME"

INSPECT_OUTPUT=$("$GOCKER" inspect "$CNAME" 2>/dev/null || true)
assert_contains "inspect returns JSON" "$INSPECT_OUTPUT" "$CNAME"

EXEC_OUTPUT=$("$GOCKER" exec "$CNAME" echo hello 2>/dev/null || true)
assert_contains "exec echo hello" "$EXEC_OUTPUT" "hello"

LOGS_OUTPUT=$("$GOCKER" logs "$CNAME" 2>/dev/null || true)
# Logs may or may not have content for sleep, just check the command succeeds
run_test "logs command succeeds" "$GOCKER" logs "$CNAME"

run_test "stop container" "$GOCKER" stop "$CNAME"

PS_A_OUTPUT=$("$GOCKER" ps -a 2>/dev/null || true)
assert_contains "stopped container shows in gocker ps -a" "$PS_A_OUTPUT" "$CNAME"

run_test "start container again" "$GOCKER" start "$CNAME"

PS_AFTER_START=$("$GOCKER" ps 2>/dev/null || true)
assert_contains "restarted container shows in gocker ps" "$PS_AFTER_START" "$CNAME"

# Small delay — Apple's container CLI needs a moment after start
sleep 2
run_test "stop container (final)" "$GOCKER" stop "$CNAME"
run_test "rm container" "$GOCKER" rm "$CNAME"

PS_AFTER_RM=$("$GOCKER" ps -a 2>/dev/null || true)
if echo "$PS_AFTER_RM" | grep -q "$CNAME"; then
    fail "container removed from gocker ps -a"
else
    pass "container removed from gocker ps -a"
fi

# ---------------------------------------------------------------------------
# 4. Interactive run
# ---------------------------------------------------------------------------

section "Interactive run"

printf "${YELLOW}  ~ skipped (cannot test TTY allocation in non-interactive script)${NC}\n"

# ---------------------------------------------------------------------------
# 5. Networks
# ---------------------------------------------------------------------------

section "Networks"

NETNAME="${PREFIX}-net"

run_test "network create" "$GOCKER" network create "$NETNAME"

# Apple's CLI may not populate the Name field — try JSON output which
# includes the raw ID, and the name we passed IS the ID.
NET_LS=$("$GOCKER" network ls --format json 2>/dev/null || "$GOCKER" network ls 2>/dev/null || true)
assert_contains "network shows in gocker network ls" "$NET_LS" "$NETNAME"

run_test "network rm" "$GOCKER" network rm "$NETNAME"

NET_LS_AFTER=$("$GOCKER" network ls 2>/dev/null || true)
if echo "$NET_LS_AFTER" | grep -q "$NETNAME"; then
    fail "network removed from gocker network ls"
else
    pass "network removed from gocker network ls"
fi

# ---------------------------------------------------------------------------
# 6. Volumes
# ---------------------------------------------------------------------------

section "Volumes"

VOLNAME="${PREFIX}-vol"

run_test "volume create" "$GOCKER" volume create "$VOLNAME"

VOL_LS=$("$GOCKER" volume ls 2>/dev/null || true)
assert_contains "volume shows in gocker volume ls" "$VOL_LS" "$VOLNAME"

run_test "volume rm" "$GOCKER" volume rm "$VOLNAME"

VOL_LS_AFTER=$("$GOCKER" volume ls 2>/dev/null || true)
if echo "$VOL_LS_AFTER" | grep -q "$VOLNAME"; then
    fail "volume removed from gocker volume ls"
else
    pass "volume removed from gocker volume ls"
fi

# ---------------------------------------------------------------------------
# 7. Compose
# ---------------------------------------------------------------------------

section "Compose"

# gocker compose proxies into a shared VM via Apple's container CLI, which
# only exists on macOS — on Linux the compose path is not wired up yet.
# GOCKER_SMOKE_SKIP_COMPOSE=1 skips it elsewhere too (e.g. networks where
# the VM cannot pull images).
if [[ "$(uname -s)" == "Linux" ]]; then
    printf "${YELLOW}  ~ skipped (compose proxies into an Apple container VM; not available on Linux)${NC}\n"
elif [[ "${GOCKER_SMOKE_SKIP_COMPOSE:-0}" == "1" ]]; then
    printf "${YELLOW}  ~ skipped (GOCKER_SMOKE_SKIP_COMPOSE=1)${NC}\n"
else

# The compose file must live under a VM-mounted workspace dir (/tmp is
# mounted by default) — macOS mktemp's default /var/folders/... is not
# visible inside the compose VM, so path translation would fail.
COMPOSE_DIR=$(mktemp -d /tmp/gocker-smoke-compose-XXXXXX)
COMPOSE_SVC="${PREFIX}-svc"

cat > "$COMPOSE_DIR/docker-compose.yml" <<EOF
services:
  ${COMPOSE_SVC}:
    image: ${SMOKE_IMAGE}
    command: sleep 300
EOF

# -f must precede the subcommand — compose args are proxied verbatim to
# nerdctl compose, which (like docker compose) rejects -f after it. For
# `logs`, trailing -f would even mean --follow.
run_test "compose up -d" "$GOCKER" compose -f "$COMPOSE_DIR/docker-compose.yml" up -d

COMPOSE_PS=$("$GOCKER" compose -f "$COMPOSE_DIR/docker-compose.yml" ps 2>/dev/null || true)
assert_contains "compose ps shows service" "$COMPOSE_PS" "$COMPOSE_SVC"

run_test "compose logs" "$GOCKER" compose -f "$COMPOSE_DIR/docker-compose.yml" logs

run_test "compose down" "$GOCKER" compose -f "$COMPOSE_DIR/docker-compose.yml" down

COMPOSE_PS_AFTER=$("$GOCKER" compose -f "$COMPOSE_DIR/docker-compose.yml" ps 2>/dev/null || true)
if echo "$COMPOSE_PS_AFTER" | grep -q "$COMPOSE_SVC"; then
    fail "compose service cleaned up after down"
else
    pass "compose service cleaned up after down"
fi

rm -rf "$COMPOSE_DIR"
COMPOSE_DIR=""

fi # end Linux compose skip

# ---------------------------------------------------------------------------
# 8. Image cleanup
# ---------------------------------------------------------------------------

section "Image cleanup"

if [[ "$SMOKE_IMAGE_PULLED" == "0" ]]; then
    # We fell back to a pre-existing cached image and the registry is
    # unreachable — removing it would leave the machine unable to restore it.
    printf "${YELLOW}  ~ skipped rmi (image was cached, not pulled by this run)${NC}\n"
# Apple CLI stores images by full reference — try both forms
elif "$GOCKER" rmi "$SMOKE_IMAGE" >/dev/null 2>&1 || \
   "$GOCKER" rmi "docker.io/library/$SMOKE_IMAGE" >/dev/null 2>&1; then
    pass "rmi $SMOKE_IMAGE"
else
    fail "rmi $SMOKE_IMAGE"
fi

# Apple CLI may retain image by digest even after rmi — check but don't
# fail the entire suite over it since rmi itself succeeded above.
if [[ "$SMOKE_IMAGE_PULLED" == "1" ]]; then
    IMAGES_AFTER=$("$GOCKER" images 2>/dev/null || true)
    if echo "$IMAGES_AFTER" | grep -q "$SMOKE_IMAGE_REPO"; then
        printf "${YELLOW}  ~ %s still visible (may be cached by digest — not a gocker bug)${NC}\n" "$SMOKE_IMAGE_REPO"
    else
        pass "$SMOKE_IMAGE_REPO removed from gocker images"
    fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

TOTAL=$((PASS_COUNT + FAIL_COUNT))
printf "\n${YELLOW}=== Summary ===${NC}\n"
printf "${GREEN}  Passed: %d${NC}\n" "$PASS_COUNT"
printf "${RED}  Failed: %d${NC}\n" "$FAIL_COUNT"
printf "  Total:  %d\n" "$TOTAL"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    printf "\n${RED}SMOKE TEST FAILED${NC}\n"
    exit 1
else
    printf "\n${GREEN}ALL SMOKE TESTS PASSED${NC}\n"
    exit 0
fi
