#!/bin/bash
set -e

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

exec "$@"
