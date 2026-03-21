# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Gocker

Docker-compatible CLI and REST API daemon that wraps Apple's `container` CLI on macOS 26+. Each container is a lightweight Linux microVM via `Virtualization.framework`. The killer feature is `gocker sandbox` — AI agent sandboxing with hardware-level isolation.

## Commands

```bash
make build          # go build with version from git tags
make install        # build + sudo cp to /usr/local/bin/
make test           # go test ./...
make lint           # golangci-lint run ./...
make template-build # build claude sandbox image with `container build`
make template-push  # build + push to docker.io/adyjay/gocker
go test ./engine/...  # run tests for a single package
```

## Architecture

```
CLI (urfave/cli v3)  ←→  Docker REST API (Unix socket ~/.gocker/gocker.sock)
         ↓                         ↓
                   engine/
         (translates to `container` CLI calls via os/exec)
                      ↓
         Apple `container` CLI (/usr/local/bin/container)
```

- **`main.go`** — entry point, holds `version` var (set via ldflags), calls `cmd.NewApp(version).Run()`
- **`cmd/`** — CLI commands via urfave/cli v3. `root.go` wires up the command tree with a shared `engine.Engine`. One file per command group.
- **`engine/`** — translation layer. `Engine` struct shells out to Apple's `container` binary. Three exec modes: `Exec()` (capture output), `ExecInteractive()` (attach TTY), `ExecStream()` (return `io.ReadCloser`).
- **`api/`** — Docker Engine REST API on Unix socket. `ServeHTTP` strips `/vX.XX/` prefixes for version-agnostic routing. Uses Go 1.22+ `http.ServeMux` with method+path patterns (`"GET /containers/json"`).
- **`sandbox/`** — AI agent sandboxing. `Manager` wraps engine with sandbox lifecycle. State persisted as JSON files in `~/.gocker/sandboxes/<name>.json`. `template.go` has built-in agent templates (claude, codex). `configsync.go` generates mount flags for host agent configs.
- **`format/`** — output formatting with `text/tabwriter`, JSON output, ID truncation, human-readable durations.

## Key Design Decisions

- **Pure shell-out, no CGo.** All operations translate to `container` CLI subcommands via `os/exec`. Single static Go binary.
- **urfave/cli v3 (not Cobra).** Uses generics-based flag access: `cmd.String("name")`, `cmd.Bool("force")`. Root is `*cli.Command`, not `*cli.App`. Action signature: `func(ctx context.Context, cmd *cli.Command) error`.
- **Flexible JSON parsing.** Apple's `container` CLI output is not yet stable. Parse functions handle both JSON arrays and newline-delimited JSON objects. Field lookups use variadic `getString(m, "id", "ID", "Id")` for case-insensitive field matching.
- **Daemon self-re-exec.** `gocker daemon start` uses `os.StartProcess` to re-exec with `--foreground` and `Setsid: true`. PID stored at `~/.gocker/daemon.pid`.
- **File-based state, no database.** Sandbox state is plain JSON files. No external dependencies beyond the `container` binary.
- **Terminal state protection.** `ExecInteractive()` saves/restores termios state via `ioctl(TIOCGETA/TIOCSETA)` so the terminal doesn't get stuck in raw mode if a `container` process crashes. Uses `syscall` directly (no CGo or external deps).
- **No Setpgid for interactive sessions.** The `container` CLI must stay in the foreground process group to manage its own TTY. Using `Setpgid: true` causes `SIGTTOU` freezes when the CLI calls `tcsetpgrp()` during process changes inside the VM. The `container` CLI handles signal forwarding internally.
- **Orphaned container cleanup.** `sandbox run` removes any container registered with Apple's `container` CLI but missing from gocker's state (caused by previous failed runs). Also cleans up on failure.

## Sandbox Template Images

- Published to `docker.io/adyjay/gocker:claude-latest` (and versioned tags like `:claude-0.2.0`)
- Dockerfile at `templates/claude/Dockerfile` — based on `python:3-slim-bookworm`
- Runs as non-root `sandbox` user (UID 1000) — Claude Code refuses `--dangerously-skip-permissions` as root
- Claude binary installed at `/home/sandbox/.local/bin/claude` (where Claude Code expects its native binary)
- Sandbox-required Claude settings (`bypassPermissions`, `skipDangerousModePermissionPrompt`) are baked into the image at `/home/sandbox/.claude/settings.json`
- Includes: Claude Code (installed directly from GCS), beads (`bd`), ripgrep, fd, git, jq, openssh-client
- `entrypoint.sh` updates Claude Code synchronously before `exec "$@"` (not in background — replacing the binary while claude is running causes SIGKILL/exit 137)
- Sandboxes get 4GB memory (`-m 4G`) — Claude Code (Node.js) gets OOM-killed with lower defaults
- GitHub Actions workflow (`.github/workflows/template-images.yml`) rebuilds weekly on Monday
- Built and pushed using `container build`/`container image push` (not Docker) — requires `container registry login docker.io`

### Config Sync Strategy

- Host `~/.claude/settings.json` is mounted read-only as `/home/sandbox/.claude/host-settings.json`
- `entrypoint.sh` merges host settings into baked-in sandbox settings using `jq`. Sandbox-required keys always win.
- Host-specific keys are stripped during merge: `hooks` (reference host paths), `sandbox` (host filesystem rules), `installedPlugins` (host-local npm paths)
- `enabledPlugins` and `extraKnownMarketplaces` ARE synced — these contain portable references (git URLs), so Claude Code fetches plugins fresh inside the sandbox
- **Do NOT mount the entire `~/.claude/` directory** — plugin configs contain absolute host paths (e.g., `/Users/adrian/.npm/...`) that break inside the container
- **`/resume` won't work in sandboxes** — sessions are stored by workspace path (`~/.claude/projects/<path>/`), and the host path (`/Users/.../gocker`) differs from the container path (`/workspace`)

## Versioning

- Version derived from git tags via `git describe --tags --always --dirty`
- Tag with `git tag vX.Y.Z` then `make install` to update
- Between tags, version includes commit count and hash (e.g., `v0.2.0-3-gabcdef`)
- Shows `-dirty` suffix for uncommitted changes

## Command Mapping (non-obvious)

- `docker rm` → `container delete` (not `container rm`)
- `docker rmi` → `container image delete`
- `docker kill` is not separate — it calls `ContainerStop`
- `docker login` → `container registry login`
- `docker tag` → `container image tag`

## Dual State Problem

Gocker maintains its own sandbox state (`~/.gocker/sandboxes/<name>.json`) separately from Apple's `container` CLI internal registry. These can get out of sync if a run fails mid-flight. The `sandbox run` command handles this by:
1. Verifying real container status via `container inspect` before trusting state files
2. Cleaning up stale state and recreating if the container is gone
3. Cleaning up orphaned containers before creating new ones
4. Cleaning up on failure after `ContainerRun` errors

When debugging "container already exists" errors, check both `gocker sandbox ls` and `container list -a`.

## Apple `container` CLI Quirks

- **No `attach` command.** Use `container exec <name> /bin/bash` instead. Gocker's `sandbox attach` wraps this.
- **Nested JSON output.** `container list --format json` returns deeply nested structures — fields like `id` and `image` are under `configuration`, not top-level. See `containerInfoFromNested()` in `engine/container.go`.
- **Core Data timestamps.** `startedDate` uses Apple's Core Data epoch (seconds since 2001-01-01), not Unix epoch.
- **No `--user` flag.** Cannot switch users at runtime; the user must be set in the Dockerfile.
- **Config mounts target `/home/sandbox/`** not `/root/` — the claude template image runs as the `sandbox` user.
- **`-m` flag for memory.** Supports K, M, G, T suffixes. Default is too low for Claude Code.

## Build Plan

See `GOCKER_PLAN.md` for the full phased implementation plan and design rationale.
