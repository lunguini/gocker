# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commit conventions

- Do not add `Co-Authored-By: Claude` trailers to commit messages in this repo.

## What is Gocker

Docker-compatible CLI and REST API daemon that wraps Apple's `container` CLI on macOS 26+ and nerdctl on Linux. Each container on macOS is a lightweight Linux microVM via `Virtualization.framework`. The killer feature is `gocker sandbox` — AI agent sandboxing with hardware-level isolation.

## Commands

```bash
make build            # go build with version from git tags
make build-linux      # cross-compile for Linux/arm64
make install          # build + sudo cp + codesign to /usr/local/bin/
make test             # go test ./...
make lint             # golangci-lint run ./...
make smoke            # end-to-end smoke test (requires container CLI)
make template-push-claude  # build + push claude sandbox image
make template-push-base    # build + push gocker-base shared VM image
make template-push         # build + push all template images
go test ./engine/...  # run tests for a single package
go test ./compose/... # run compose tests
```

## Setup Wizard

`gocker setup` is the first-run flow. It installs Apple Container CLI and then runs an interactive configuration wizard:

- **Isolation mode** — full / hybrid / shared (see Key Design Decisions). Explanations printed before the prompt.
- **VM resources** — CPU/memory, defaulted from host specs and the chosen isolation mode.
- **Shell integration** (opt-in) — writes `DOCKER_HOST` and `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE` to `~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish` inside sentinel-marked blocks. Idempotent; skips if existing exports already point at the gocker socket.
- **Docker context** (opt-in) — creates a `gocker` docker context and makes it active.

Non-interactive mode: `gocker setup --yes` uses `shared` isolation (the CI-friendly default), writes `~/.gocker/config.yaml`, and skips all shell/dotfile/docker-context modifications.

Re-running `gocker setup` is safe: existing config is preserved unless the user picks a different answer. Shell blocks use `# >>> gocker setup >>>` / `# <<< gocker setup <<<` markers for clean diffing and removal.

## Architecture

```
CLI (urfave/cli v3)  ←→  Docker REST API (Unix socket ~/.gocker/gocker.sock)
         ↓                         ↓
              engine.Runtime interface
         ↙              ↓              ↘
   Engine          NerdctlRuntime    SharedVMRuntime
 (Apple CLI)        (nerdctl)      (proxies into shared VM)
      ↓                 ↓                    ↓
 container CLI     nerdctl CLI      container exec gocker-shared gocker ...
  (macOS)           (Linux)              (hybrid/shared mode)
```

- **`main.go`** — entry point, holds `version` var (set via ldflags), calls `cmd.NewApp(version).Run()`
- **`cmd/`** — CLI commands via urfave/cli v3. `root.go` wires up the command tree with isolation-aware runtime routing. One file per command group.
- **`engine/`** — runtime abstraction layer:
  - `runtime.go` — `Runtime` interface (27 methods covering containers, images, networks, volumes)
  - `engine.go` — Apple Container CLI backend (macOS)
  - `nerdctl.go` — containerd/nerdctl backend (Linux)
  - `detect.go` — auto-detects runtime based on `runtime.GOOS`
  - `term_darwin.go` / `term_linux.go` — platform-specific terminal save/restore
  - `container.go`, `image.go`, `network.go`, `volume.go` — Apple CLI parsers (handle nested JSON, Core Data timestamps)
- **`config/`** — YAML config loader (`~/.gocker/config.yaml`). Isolation mode resolution with priority: CLI flag > subsystem > global > default.
- **`sharedvm/`** — shared VM for hybrid/shared isolation modes:
  - `manager.go` — VM lifecycle (EnsureRunning, Stop, Remove)
  - `runtime.go` — `SharedVMRuntime` implementing `Runtime` by proxying commands via `container exec gocker-shared gocker ...`
  - `mounts.go` — host→VM path translation for workspace mounts
  - `state.go` — VM state persistence at `~/.gocker/sharedvm/state.json`
- **`compose/`** — compose orchestration:
  - `parser.go` — YAML parsing with env var substitution, dependency ordering (Kahn's algorithm)
  - `orchestrator.go` — service lifecycle (Up/Down/Ps/Logs/Restart), handles ext4 lost+found via PGDATA injection
  - `project.go` — project state at `~/.gocker/compose/<project>/state.json`
  - `yaml.go` — custom unmarshalers for flexible compose syntax (command as string/list, env as map/list)
- **`api/`** — Docker Engine REST API on Unix socket. `ServeHTTP` strips `/vX.XX/` prefixes for version-agnostic routing. Uses Go 1.22+ `http.ServeMux` with method+path patterns (`"GET /containers/json"`). `logging.go` provides rolling terminal display (last N lines) and file-based request logging for `--foreground` mode.
- **`sandbox/`** — AI agent sandboxing. `Manager` wraps runtime with sandbox lifecycle. State persisted as JSON files in `~/.gocker/sandboxes/<name>.json`. `template.go` has built-in agent templates (claude, codex). `configsync.go` generates mount flags for host agent configs.
- **`format/`** — output formatting with `text/tabwriter`, JSON output, ID truncation, human-readable durations.

## Docker Alias

The user has `docker` aliased to `gocker` on this system. When tools or scripts call `docker`, they are actually invoking gocker. This means gocker must accept Docker CLI flags and arguments even if the underlying feature isn't fully implemented — unknown flags should be accepted gracefully (ignored with a warning) rather than causing hard errors.

## Key Design Decisions

- **Pure shell-out, no CGo.** All operations translate to CLI subcommands via `os/exec`. Single static Go binary.
- **Container binary resolution (macOS).** Priority: `runtimeBinary` in config > `exec.LookPath("container")` > `/usr/local/bin/container` fallback (kept last because GUI-launched processes can have a minimal PATH). See `resolveContainerBinary` in `engine/detect.go`. Never hardcode the binary path elsewhere.
- **Atomic state writes.** All JSON/YAML state files (sandbox, sharedvm, compose, config) go through `internal/fsutil.WriteFileAtomic` (temp file + rename) so a crash mid-write can't corrupt state. New state persistence must use it, not bare `os.WriteFile`. Likewise use `fsutil.HomeDir()` (fails fast with a clear message) instead of `home, _ := os.UserHomeDir()` when building `~/.gocker` paths — the silent variant produces paths rooted at `/` when `$HOME` is unset.
- **State write serialization.** Save/delete of sandbox, sharedvm, and compose state is wrapped in `fsutil.WithLock(lockPath, fn)` — an `flock(LOCK_EX)` advisory lock on a per-area lock file (e.g. `~/.gocker/sandboxes.lock`) — so concurrent gocker invocations don't race on read-modify-write of the same state directory. Atomic writes stop torn files; the lock stops lost updates. New state-mutation paths should acquire the same per-area lock.
- **`gocker doctor`.** Diagnostics command (`cmd/doctor.go`) reports platform, config path/validity, effective isolation mode, container binary resolution (with source: config/PATH/fallback via `engine.ResolveBinaryInfo`), and daemon socket health. Failing checks exit non-zero; warnings are advisory. `renderDiagnostics` is a pure formatter kept separate from `gatherDiagnostics` (which does the IO) so the output logic is unit-testable.
- **API not-found mapping.** Runtime errors in `api/` handlers go through `writeRuntimeError` (`api/errors.go`), which string-matches known "doesn't exist" phrasings from Apple CLI/nerdctl and returns Docker-style 404s. Add new phrasings to `isNotFoundErr`, not inline in handlers.
- **Runtime interface.** `engine.Runtime` abstracts over Apple Container CLI (macOS) and nerdctl (Linux). `SharedVMRuntime` proxies commands into a persistent VM for hybrid/shared isolation modes.
- **urfave/cli v3 (not Cobra).** Uses generics-based flag access: `cmd.String("name")`, `cmd.Bool("force")`. Root is `*cli.Command`, not `*cli.App`. Action signature: `func(ctx context.Context, cmd *cli.Command) error`.
- **Flexible JSON parsing.** Apple's `container` CLI output is not yet stable. Parse functions handle both JSON arrays and newline-delimited JSON objects. Field lookups use variadic `getString(m, "id", "ID", "Id")` for case-insensitive field matching.
- **Isolation modes.** `full` (every container = own VM), `hybrid` (compose/run share a VM, sandboxes get dedicated VMs), `shared` (everything in one VM). Configured via `~/.gocker/config.yaml` or `--isolation` flag.
- **Recursive gocker.** The shared VM runs gocker itself on Linux (with nerdctl backend). Commands are proxied via `container exec gocker-shared gocker <args>`.
- **Daemon self-re-exec.** `gocker daemon start` uses `os.StartProcess` to re-exec with `--foreground` and `Setsid: true`. PID stored at `~/.gocker/daemon.pid`.
- **File-based state, no database.** Sandbox, compose, and shared VM state are plain JSON files. No external dependencies beyond the container runtime binary.
- **Terminal state protection.** `ExecInteractive()` saves/restores termios state via platform-specific ioctl so the terminal doesn't get stuck in raw mode if a process crashes.
- **No Setpgid for interactive sessions.** The `container` CLI must stay in the foreground process group to manage its own TTY. Using `Setpgid: true` causes `SIGTTOU` freezes when the CLI calls `tcsetpgrp()` during process changes inside the VM. The `container` CLI handles signal forwarding internally.
- **Orphaned container cleanup.** `sandbox run` and `compose up` remove any container registered with the CLI but missing from gocker's state (caused by previous failed runs). Also cleans up on failure.
- **Flag passthrough architecture.** The Runtime interface accepts raw `[]string` args — backends forward them directly to the underlying CLI binary. Feature gaps are almost always in gocker's CLI layer (`cmd/`), not the backend. Adding a flag to `cmd/run.go` is usually sufficient; nerdctl will handle it on Linux, Apple CLI support varies.
- **Shared/hybrid mode uses standard container isolation.** In shared/hybrid modes, containers run inside the VM via nerdctl/containerd with standard namespace/cgroup isolation — same as Docker. Only full mode provides hardware VM boundaries per container.
- **Compose proxies to nerdctl, not reimplemented.** Rather than maintaining a custom compose orchestrator, gocker proxies all `compose` commands to `nerdctl compose` inside the shared VM. This gives full Docker Compose compatibility (multi-file, build, profiles, etc.) for free. Raw args are passed through via `SkipFlagParsing` — no flag-by-flag reconstruction. Host paths are translated to VM-internal paths (`/host/...`).
- **SkipFlagParsing for passthrough commands.** Commands that proxy to another tool (compose, exec) should use `SkipFlagParsing: true` and parse only known flags manually. Otherwise urfave/cli rejects unknown flags (e.g., `bash -c "cmd"` where `-c` is treated as a flag).
- **Apple `container exec` TTY rules.** Use `-i` (not `-t`) for the outer `container exec` into the VM when the subprocess may not have a TTY. `-t` fails with "Operation not supported by device" when stdin is not a terminal. `-i` alone works for both interactive and non-interactive use. Never combine outer `-t` with inner nerdctl `-T` (no-TTY).
- **BuildKit runs inside the VM.** BuildKit is Linux-only — no native macOS build. `buildkitd` runs inside the shared VM alongside containerd. The gocker-base image includes BuildKit binaries. Don't use `--oci-worker-no-process-sandbox` (requires rootless mode); plain `buildkitd` works as root.
- **cgroup v2 delegation for nested containers.** The VM needs cgroup v2 delegation configured in the init script: move processes out of root cgroup into `/sys/fs/cgroup/init`, then enable `+cpuset +cpu +io +memory +pids` on the root's `subtree_control`. Without this, runc fails with "cannot enter cgroupv2 with domain controllers".
- **Daemon must use isolation-aware runtime.** The API daemon (`gocker daemon start`) must receive the `SharedVMRuntime` in shared/hybrid mode, not the raw `appleRT`. Otherwise API calls (container list, exec, inspect) can't see containers running inside the VM.
- **Docker API type mismatches.** Docker SDK clients are strict about JSON types. Image inspect `Created` must be an RFC3339 string (not Unix int). Container/network/volume inspect must return a JSON object (not an array) with capitalized field names. Image delete must return 404 (not 500) for missing images. Empty list endpoints must return `[]`, not `null`. Apple CLI often returns arrays with lowercase keys — reshape via `handle*Inspect` before writing to the response.
- **Docker SDK compatibility tests.** `api/docker_compat_test.go` decodes every JSON-returning endpoint into the real Docker SDK types (`types.NetworkResource`, `volume.Volume`, `container.CreateResponse`, etc.). When adding a new API endpoint that returns JSON, add a matching test there — if the SDK can unmarshal it, real clients (including `docker` CLI via context) can too. `github.com/docker/docker@v26.1.5+incompatible` is pinned as a test-only dep; avoid upgrading past v26 because later versions rename the `/api` submodule path and break `go mod tidy`.
- **Docker Compose network ownership.** Compose v2 refuses to adopt pre-existing networks that lack the `com.docker.compose.network=<name>` / `com.docker.compose.project=<project>` labels (error: "network X was found but has incorrect label"). This matches Docker; not a gocker bug. Users must either mark the network `external: true` in the compose file, or let compose create it from scratch.
- **nerdctl vs Docker flag differences.** nerdctl compose doesn't support `--rmi` (compose down) or `--wait` (compose up). These must be stripped before forwarding. When adding Docker compat flags, check nerdctl support and silently drop unsupported ones.

## Template Images

### Claude Sandbox Image
- Published to `docker.io/adyjay/gocker:claude-latest` (and versioned tags like `:claude-0.2.0`)
- Dockerfile at `templates/claude/Dockerfile` — based on `python:3-slim-bookworm`
- Runs as non-root `sandbox` user (UID 1000) — Claude Code refuses `--dangerously-skip-permissions` as root
- Claude binary installed at `/home/sandbox/.local/bin/claude` (where Claude Code expects its native binary)
- Sandbox-required Claude settings (`bypassPermissions`, `skipDangerousModePermissionPrompt`) are baked into the image at `/home/sandbox/.claude/settings.json`
- Includes: Claude Code (installed directly from GCS), beads (`bd`), ripgrep, fd, git, jq, openssh-client
- Claude Code handles its own auto-updates at startup — the entrypoint only merges settings, no manual update logic
- Sandboxes get 4GB memory (`-m 4G`) — Claude Code (Node.js) gets OOM-killed with lower defaults

### Gocker-Base Image (Shared VM)
- Published to `docker.io/adyjay/gocker:base-latest`
- Dockerfile at `templates/base/Dockerfile` — based on `debian:bookworm-slim`
- Contains: containerd, runc, nerdctl, CNI plugins, gocker (Linux build)
- `gocker-init.sh` starts containerd, then keeps the container alive for `exec` commands
- Used by hybrid/shared isolation modes as the persistent shared VM

### Config Sync Strategy (Claude Sandbox)
- Host `~/.claude/settings.json` is mounted read-only as `/home/sandbox/.claude/host-settings.json`
- `entrypoint.sh` merges host settings into baked-in sandbox settings using `jq`. Sandbox-required keys always win.
- Host-specific keys are stripped during merge: `hooks` (reference host paths), `sandbox` (host filesystem rules), `installedPlugins` (host-local npm paths)
- `enabledPlugins` and `extraKnownMarketplaces` ARE synced — these contain portable references (git URLs), so Claude Code fetches plugins fresh inside the sandbox
- **Do NOT mount the entire `~/.claude/` directory** — plugin configs contain absolute host paths (e.g., `/Users/adrian/.npm/...`) that break inside the container
- **`/resume` won't work in sandboxes** — sessions are stored by workspace path (`~/.claude/projects/<path>/`), and the host path (`/Users/.../gocker`) differs from the container path (`/workspace`)

### GitHub Actions
- `.github/workflows/template-images.yml` rebuilds both images weekly on Monday
- Claude image pushed to Docker Hub, base image pushed to Docker Hub
- Manual trigger via `workflow_dispatch`

## Testing

```bash
make test     # unit + golden file tests (no container CLI needed)
make smoke    # full end-to-end (requires macOS 26+ with container CLI)
```

- **Golden file tests** (`engine/testdata/`) — captured Apple CLI JSON output tested against parsers. When Apple changes the format, update testdata and fix failing tests.
- **Compose unit tests** (`compose/`) — YAML parsing, dependency ordering, volume resolution, env injection.
- **Smoke test** (`test/smoke.sh`) — exercises every CLI interaction: pull, run, ps, inspect, exec, logs, stop, rm, networks, volumes, compose up/down.
- **E2E compose tests** (`test/e2e/`) — real services via `gocker compose`. Run `make e2e` before tagging a release. Each scenario is a self-contained `test/e2e/scenarios/<name>/` directory with `docker-compose.yml` + `assert.sh`. Takes 5-10 minutes and pulls images from Docker Hub.

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

Gocker maintains its own state (`~/.gocker/sandboxes/`, `~/.gocker/compose/`, `~/.gocker/sharedvm/`) separately from Apple's `container` CLI internal registry. These can get out of sync if a run fails mid-flight. The commands handle this by:
1. Verifying real container status via `container inspect` before trusting state files
2. Cleaning up stale state and recreating if the container is gone
3. Cleaning up orphaned containers before creating new ones
4. Cleaning up on failure after `ContainerRun` errors

When debugging "container already exists" errors, check both `gocker sandbox ls` / `gocker compose ps` and `container list -a`.

## Apple `container` CLI Quirks

- **No `attach` command.** Use `container exec <name> /bin/bash` instead. Gocker's `sandbox attach` wraps this.
- **Nested JSON output.** `container list --format json` returns deeply nested structures — fields like `id` and `image` are under `configuration`, not top-level. See `containerInfoFromNested()` in `engine/container.go`.
- **Core Data timestamps.** `startedDate` uses Apple's Core Data epoch (seconds since 2001-01-01), not Unix epoch.
- **No `--user` flag.** Cannot switch users at runtime; the user must be set in the Dockerfile.
- **Config mounts target `/home/sandbox/`** not `/root/` — the claude template image runs as the `sandbox` user.
- **`-m` flag for memory.** Supports K, M, G, T suffixes. Default is too low for Claude Code.
- **ext4 volumes include `lost+found`.** Unlike Docker's volume driver, Apple's `container volume create` formats with ext4. Gocker auto-injects `PGDATA` for PostgreSQL and `MYSQL_DATADIR` for MySQL/MariaDB to work around this.
- **Mounts only at creation time.** Apple Container only accepts `-v` flags during `container run`. No dynamic mount command. The shared VM pre-mounts the user's home directory.

## Design Documents

- `.claude/GOCKER_ISOLATION_ADDENDUM.md` — isolation modes, shared VM architecture, cross-platform strategy, recursive gocker design
- `GOCKER_PLAN.md` — original phased implementation plan
