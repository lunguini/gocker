# TTY-Aware Sandbox + Dynamic Mount Expansion

**Date:** 2026-04-12
**Status:** Approved

## Problem

External harnesses (e.g., Crush, a bubbletea-based tool) cannot use gocker for container execution:

1. **`gocker sandbox run` fails with ENODEV** — the sandbox manager hardcodes `-i -t` flags. When the harness captures the terminal (no real TTY on stdin), Apple Container CLI's internal `tcsetpgrp()` call fails with ENODEV.
2. **`gocker run` bind mounts don't propagate** — in shared/hybrid isolation modes, `TranslatePath()` silently returns the original host path when it falls outside configured `workspaceDirs`. The VM doesn't have that path mounted, so the container sees an empty/missing mount.

## Approach

**TTY detection** — check whether stdin is a terminal before passing `-t` to the container CLI. **Dynamic mount expansion** — when a bind mount path isn't covered by existing VM mounts, automatically recreate the VM with the additional mount.

## Isolation Mode Coverage

The two fixes apply differently depending on isolation mode:

| Mode | `sandbox run` ENODEV | `sandbox run` mounts | `gocker run` mounts |
|------|---------------------|---------------------|-------------------|
| **Full** | Affected | Works (direct Apple CLI) | Works (direct Apple CLI) |
| **Hybrid** | Affected | Works (direct Apple CLI) | Affected (SharedVMRuntime) |
| **Shared** | Affected | Affected (SharedVMRuntime) | Affected (SharedVMRuntime) |

- **TTY fix**: Applies to all isolation modes — the ENODEV comes from the Apple Container CLI regardless of how it's invoked.
- **Mount expansion**: Only applies when commands route through `SharedVMRuntime` (hybrid for `gocker run`, shared for everything). In full isolation mode, each container is its own VM and `-v` flags go directly to Apple Container CLI — mounts work natively with no translation needed.

The mount expansion code lives entirely in `sharedvm/` and `SharedVMRuntime`, so it is naturally scoped to only the modes that need it. Full isolation mode is unaffected.

## Design

### 1. TTY Detection

Add an `IsTerminal()` helper that probes stdin with `TIOCGETA` (macOS) / `TCGETS` (Linux) and returns a bool. This uses the same syscall already in `saveTermState()` but returns a simple boolean instead of termios state.

**Sandbox manager flag logic** (replaces hardcoded `-i -t` in `sandbox/manager.go`):
- TTY present + not detached: `-i -t` (current behavior, interactive session)
- No TTY + not detached: `-i` only (keeps stdin open for piped input, no TTY allocation)
- Detached: `-d` only (background mode, no `-i` or `-t`)

`ExecInteractive` in `engine/engine.go` needs no changes — `saveTermState()` already returns nil gracefully when stdin isn't a terminal. The fix is preventing `-t` from reaching the container CLI.

### 2. Mount Path Translation — Error Surfacing

Change `TranslatePath` signature from `func(string, map[string]string) string` to `func(string, map[string]string) (string, bool)` — the bool indicates whether translation succeeded (the path was covered by an existing mount).

Change `TranslateVolumeSpec` to return `(string, error)` with a clear error message naming the unmapped path.

### 3. Mount Parent Resolution

When an unmapped path is detected, resolve the mount directory:
- If the source is a directory, mount that directory exactly
- If the source is a file, mount its immediate parent directory
- **Blocklist**: Never auto-mount broad system roots (`/`, `/tmp`, `/var`, `/etc`, `/private`). If the resolved parent is one of these, error with guidance to use a more specific path.

Example: `/tmp/crush-abc123/file.txt` → mount `/tmp/crush-abc123`, not `/tmp`.

### 4. Dynamic VM Mount Expansion

New `ExpandMounts(ctx context.Context, paths []string) error` method on `sharedvm.Manager`:

1. Filter out paths already covered by existing mounts
2. Check if containers are running inside the VM — if so, error: `"Cannot expand mounts while containers are running in the shared VM. Stop them first, or add the path to workspaceDirs in ~/.gocker/config.yaml"`
3. Stop and remove the VM
4. Add new mount entries to `manager.mounts`
5. Recreate the VM with expanded mount set
6. Wait for readiness (same probe loop as `EnsureRunning`)
7. Persist expanded mounts in `VMState` (`~/.gocker/sharedvm/state.json`)
8. Print: `"Recreating shared VM to add mount for /path..."`

### 5. ContainerRun Integration

In `SharedVMRuntime.ContainerRun` (and `ImageBuild`):

1. Call `translateMountArgs` — collect any paths that failed translation
2. For each failed path, compute the mount parent per resolution rules
3. Call `manager.ExpandMounts(ctx, newMountDirs)`
4. Retry `translateMountArgs` — should now succeed
5. Proceed with the original container run

## File Changes

| File | Change |
|------|--------|
| `engine/tty.go` (new) | `IsTerminal()` — `TIOCGETA` probe on stdin fd, returns bool |
| `engine/tty_linux.go` (new) | Linux equivalent using `TCGETS` |
| `sandbox/manager.go` | TTY-aware flag logic replacing hardcoded `-i -t` |
| `sharedvm/mounts.go` | `TranslatePath` returns `(string, bool)`. `TranslateVolumeSpec` returns `(string, error)`. New `ResolveMountParent()` with blocklist. |
| `sharedvm/manager.go` | New `ExpandMounts(ctx, paths)` method |
| `sharedvm/runtime.go` | `ContainerRun` and `ImageBuild`: detect translation failures, trigger `ExpandMounts`, retry |
| `sharedvm/mounts_test.go` | Tests for translation errors, parent resolution, blocklist |

## What Doesn't Change

- `engine/engine.go` `ExecInteractive` — already handles nil `saveTermState` gracefully
- `engine/runtime.go` `Runtime` interface — no signature changes
- `cmd/run.go` — TTY detection for `gocker run` already works via the `interactive` bool from CLI `-i`/`-t` flags; the fix targets the sandbox manager which hardcodes them
- Full isolation mode mount paths — unaffected, direct Apple CLI
