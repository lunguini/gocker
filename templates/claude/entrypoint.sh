#!/bin/bash
set -e

GCS_BUCKET="https://storage.googleapis.com/claude-code-dist-86c565f3-f756-42ad-8dfa-d59b1c096819/claude-code-releases"
CLAUDE_BIN="$HOME/.local/bin/claude"
SETTINGS="$HOME/.claude/settings.json"
HOST_SETTINGS="$HOME/.claude/host-settings.json"

# Merge host settings into baked-in sandbox settings if mounted.
# Carries over: enabledPlugins, extraKnownMarketplaces, env, etc.
# Strips: hooks (host paths), sandbox rules (host paths),
#         installedPlugins (host-local installs — Claude will re-fetch from marketplaces).
# Sandbox-required keys always win.
if [ -f "$HOST_SETTINGS" ]; then
    merged=$(jq -s '
        (.[0] | del(.hooks, .sandbox, .installedPlugins)) * .[1] * {
            skipDangerousModePermissionPrompt: true,
            permissions: { defaultMode: "bypassPermissions" }
        }
    ' "$HOST_SETTINGS" "$SETTINGS" 2>/dev/null) && \
    echo "$merged" > "$SETTINGS"
fi

# Compare semver strings. Returns 0 if $1 > $2.
version_gt() {
    [ "$(printf '%s\n%s' "$1" "$2" | sort -V | head -1)" != "$1" ]
}

# Update Claude Code if a newer version is available.
# Runs synchronously — replacing the binary while claude is running causes SIGKILL.
update_claude() {
    ARCH=$(uname -m | sed 's/aarch64/arm64/' | sed 's/x86_64/x64/')
    LATEST=$(curl -fsSL --connect-timeout 3 "$GCS_BUCKET/latest" 2>/dev/null) || return 0
    CURRENT=$("$CLAUDE_BIN" --version 2>/dev/null | head -1) || CURRENT="0.0.0"

    if [ -n "$LATEST" ] && version_gt "$LATEST" "$CURRENT"; then
        echo "Updating Claude Code from $CURRENT to $LATEST..."
        curl -fsSL -o /tmp/claude-update "$GCS_BUCKET/$LATEST/linux-$ARCH/claude" 2>/dev/null \
            && chmod +x /tmp/claude-update \
            && mv /tmp/claude-update "$CLAUDE_BIN" \
            && echo "Claude Code updated to $LATEST"
    fi
}

update_claude

exec "$@"
