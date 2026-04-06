# Changelog

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
