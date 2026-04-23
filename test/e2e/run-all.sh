#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

log_section "Gocker E2E — running all scenarios"
log_info "using binary: $("$GOCKER" --version 2>&1 | head -1)"

SCENARIOS=()
for dir in "$SCRIPT_DIR"/scenarios/*/; do
    [ -d "$dir" ] || continue
    SCENARIOS+=("$(basename "$dir")")
done

if [ "${#SCENARIOS[@]}" -eq 0 ]; then
    log_fail "no scenarios found under $SCRIPT_DIR/scenarios/"
    exit 2
fi

pass_list=()
fail_list=()

for scenario in "${SCENARIOS[@]}"; do
    if "$SCRIPT_DIR/run.sh" "$scenario"; then
        pass_list+=("$scenario")
    else
        fail_list+=("$scenario")
    fi
done

log_section "Summary"
printf "Passed: %d\n" "${#pass_list[@]}"
for s in "${pass_list[@]}"; do printf "  ${GREEN}✓${NC} %s\n" "$s"; done
printf "Failed: %d\n" "${#fail_list[@]}"
for s in "${fail_list[@]}"; do printf "  ${RED}✗${NC} %s\n" "$s"; done

[ "${#fail_list[@]}" -eq 0 ] || exit 1
