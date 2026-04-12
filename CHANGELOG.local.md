# Changelog

## Unreleased

### Harness compatibility (TTY-aware sandbox + dynamic mount expansion)

- fix: `gocker sandbox run` no longer fails with ENODEV when invoked without a TTY (e.g., from external harnesses like Crush)
- feat: TTY detection — sandbox skips `-t` flag when stdin is not a terminal, uses `-i` only for piped input
- feat: dynamic mount expansion — bind mount paths outside configured `workspaceDirs` are auto-detected and the shared VM is recreated with the additional mounts
- feat: mount path safety — broad system directories (`/`, `/tmp`, `/var`, `/etc`, `/private`) are blocked from auto-mounting
- feat: clear error messages when mount paths cannot be resolved
- fix: `TranslatePath` and `TranslateVolumeSpec` now surface errors instead of silently returning untranslated paths

### Compose: Full Docker Compose compatibility via nerdctl proxy

- feat: proxy all compose commands to `nerdctl compose` inside the shared VM, giving full Docker Compose compatibility (multi-file, build, profiles, extends, exec, cp, etc.) without maintaining a custom orchestrator
- feat: add `compose build` support via BuildKit inside the shared VM
- feat: add per-project VMs for compose in full isolation mode (`gocker-compose-<project>`)
- feat: support all compose subcommands (up, down, build, ps, logs, exec, run, stop, start, pull, config, top, images, restart, cp)
- feat: forward and translate host environment variables into the VM for compose file variable substitution
- feat: translate host paths in compose args (`-f`, `--project-directory`, `cp` source paths) to VM-internal paths
- feat: add `-T` (no-TTY) support for non-interactive compose exec
- feat: accept Docker-only flags (`--rmi`, `--wait`) gracefully (stripped before forwarding to nerdctl)
- feat: raw arg passthrough via `SkipFlagParsing` — all Docker Compose flags are forwarded without needing explicit definitions

### Docker API compatibility fixes

- fix: image routes use `{name...}` rest patterns for slashed image names (e.g., `alexgshaw/image:tag`)
- fix: image inspect returns `Created` as RFC3339 string instead of Unix timestamp (Docker SDK expects string)
- fix: image delete returns 404 for missing images instead of 500
- fix: container inspect handles Apple CLI's empty array `[]` response (returns 404 instead of 500)
- fix: container inspect unwraps Apple CLI's JSON array responses into single objects
- feat: add top-level `gocker info` command (alias for `system info`) for Docker CLI compatibility

### Daemon logging

- feat: rolling 5-line terminal display for daemon foreground mode with ANSI cursor control
- feat: persistent request log at `~/.gocker/daemon.log`
- feat: log error response bodies for 4xx/5xx responses to aid debugging

### CLI improvements

- fix: `gocker exec` uses `SkipFlagParsing` to handle `bash -c "command"` correctly (previously `-c` was treated as a flag)
- feat: compose persistent flags (`-f`, `-p`, `--project-directory`) propagate to all subcommands via urfave/cli v3 flag inheritance

### SharedVM / Architecture

- feat: configurable VM names via `NewManagerWithName()` for per-project compose VMs
- feat: add BuildKit (`buildkitd` + `buildctl`) to the gocker-base VM image
- feat: cgroup v2 delegation setup in VM init script for nested container support
- feat: daemon API routes through `SharedVMRuntime` in shared/hybrid mode (previously always used host runtime)
- feat: `docker` symlink to `gocker` at `/usr/local/bin/docker` for subprocess compatibility

### Documentation

- feat: add `docs/docker-compatibility.md` — full Docker CLI and API compatibility matrix
- docs: update CLAUDE.md with flag passthrough architecture, shared/hybrid isolation details, Docker alias note, logging docs, and key architectural insights from harbor integration work

## v0.5.0

### Shared VM Improvements
- **Fix: containers and images invisible in shared mode** — JSON parsers now handle both JSON arrays (from gocker's `--format json`) and newline-delimited JSON objects (from nerdctl), fixing `gocker ps`, `gocker images`, `gocker network ls`, and `gocker volume ls` returning empty results when using shared/hybrid isolation
- **Fix: `--format json` flag positioning** — root-level `--format` flag was incorrectly placed after subcommands when proxying into the shared VM, causing urfave/cli to ignore it
- **Shared VM port visibility** — `gocker ps` now rewrites `0.0.0.0` in port bindings to the actual VM IP address, making ports directly usable from the host. `gocker run -p` prints the VM IP when publishing ports in shared mode
- **VM IP discovery** — new `Manager.VMIP()` method extracts the shared VM's IP from Apple Container inspect output

### Docker API in Shared VM
- **Gocker daemon auto-starts inside the shared VM** — `gocker-init.sh` now launches `gocker daemon` with a Docker-compatible socket at `/var/run/docker.sock`, enabling tools like Portainer to manage containers inside the VM
- **`--socket` flag for `gocker daemon start`** — allows specifying a custom socket path (defaults to `~/.gocker/gocker.sock`)

### Bug Fixes
- Prevent shared VM recreation on transient inspect failures
- Fix installer to find latest release with actual assets
- Resolve golangci-lint v2 errcheck and staticcheck warnings

### Testing & CI
- Integration tests for system, container, sharedvm, and nerdctl
- Unit tests for VM state persistence, EnsureRunning, EnsureSystemRunning
- MockRuntime for unit testing Runtime consumers
- CI workflow with Apple Container CLI installation

## v0.4.3

- fix: brew formula incorrectly installing go as a dependency chore: update docs to showcase brew installation

## v0.4.2

- fix: template update mechanism conflicting with claude code's update mechanism
- fix: update deps

## v0.4.1

- fix: sandbox using subsystem instead of sandbox config

## v0.4.0

### Compose Support
- `gocker compose up/down/ps/logs/restart` with standard `docker-compose.yml` files
- Dependency ordering (topological sort) for service startup
- Environment variable substitution (`${VAR:-default}`) and `.env` file support
- Named volume support with automatic ext4 `lost+found` workaround for PostgreSQL/MySQL
- Project-scoped networks, volumes, and container naming

### Isolation Modes
- Three configurable modes: `full` (default), `hybrid`, `shared`
- `full`: every container is its own Apple Container microVM
- `hybrid`: compose and run share a persistent VM; sandboxes get dedicated microVMs
- `shared`: everything in one VM (with safety warning for sandboxes)
- Per-command override via `--isolation` flag
- Shared VM lifecycle management: `gocker daemon vm status/stop/rm`

### Cross-Platform Runtime
- `engine.Runtime` interface abstracting over container backends
- Apple Container CLI backend (macOS) — existing behavior
- NerdctlRuntime backend (Linux) — gocker works natively on Linux with containerd/nerdctl
- Auto-detection based on platform (`runtime.GOOS`)
- Platform-specific terminal state management (`term_darwin.go`, `term_linux.go`)

### Configuration
- YAML config file at `~/.gocker/config.yaml`
- Isolation mode, shared VM settings, runtime override, workspace directories
- Per-subsystem isolation overrides (compose, sandbox)

### Docker API Compatibility
- Portainer, lazydocker, and Testcontainers work via `~/.gocker/gocker.sock`
- Socket mount: `-v ~/.gocker/gocker.sock:/var/run/docker.sock`

### Claude Code Session Sync
- Sandbox sessions persist back to the host — `/resume` works across host and sandbox
- Automatic session directory mapping between host and VM workspace paths
- Configurable via `sandbox.syncClaudeSession` in config (default: enabled)

### AI Agent Integration
- `gocker ai` — outputs complete CLI reference optimized for AI agent consumption
- Includes workspace context detection (compose files, Dockerfiles, running containers)

### Shell Completions
- `gocker completion bash/zsh/fish` — built-in shell completion generation

### Template Images
- Gocker-base image (`adyjay/gocker:base-latest`) for shared VM with containerd + nerdctl + gocker
- Restructured Makefile: `template-push-claude`, `template-push-base`, `template-push` (all)
- `build-linux` target for cross-compiling gocker for Linux/arm64

### Testing
- End-to-end smoke test suite (`make smoke`) — exercises every CLI interaction
- Golden file parser tests for Apple Container CLI output format changes
- Compose unit tests — YAML parsing, dependency ordering, volume resolution, env injection
- Performance benchmarks (`make benchmark`) — gocker vs Docker Desktop comparison

### CI/CD
- GoReleaser configuration for cross-platform binary releases
- GitHub Actions release workflow — triggered on version tags
- Template images workflow — versioned tags on release, weekly `:latest` rebuilds
- Homebrew formula auto-generation via GoReleaser tap integration

### Other
- `--restart` flag accepted with graceful warning (not supported by Apple Container CLI)
- `--hostname` flag support
- Code signing in `make install` to prevent macOS SIGKILL
