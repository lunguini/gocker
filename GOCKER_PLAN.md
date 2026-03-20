# Gocker — Docker-compatible CLI + API for Apple Container on macOS

## What is this document

This is the build plan for **gocker**, a Go CLI and Docker-compatible API daemon that wraps Apple's `container` CLI (apple/container) to provide full Docker ecosystem compatibility on macOS 26+ with Apple Container's microVM-per-container architecture. The killer feature is `gocker sandbox` — AI agent sandboxing with hardware-level isolation.

This document is intended to be used as `CLAUDE.md` in a Claude Code session to build the project from scratch.

## Repository

- **GitHub org**: `lunguini`
- **Repo name**: `gocker`
- **Module path**: `github.com/lunguini/gocker`
- **License**: Apache 2.0
- **Go version**: 1.26+

---

## Architecture Overview

```
┌─────────────────────────────────────────────────┐
│  User / Developer                               │
│                                                  │
│  gocker run ...          lazydocker              │
│  gocker sandbox run ...  Portainer               │
│  gocker compose up       Testcontainers          │
│       │                     │                    │
│       ▼                     ▼                    │
│  ┌─────────┐    ┌──────────────────────┐         │
│  │  CLI    │    │  Docker REST API     │         │
│  │ (urfave)│    │  (Unix socket)       │         │
│  └────┬────┘    │  ~/.gocker/gocker.sock│        │
│       │         └──────────┬───────────┘         │
│       ▼                    ▼                     │
│  ┌──────────────────────────────────────┐        │
│  │          gocker engine               │        │
│  │  (translates to `container` CLI)     │        │
│  └──────────────────┬───────────────────┘        │
│                     ▼                            │
│  ┌──────────────────────────────────────┐        │
│  │   Apple `container` CLI              │        │
│  │   (backed by Containerization.framework)│     │
│  └──────────────────┬───────────────────┘        │
│                     ▼                            │
│  ┌──────────────────────────────────────┐        │
│  │   macOS Virtualization.framework     │        │
│  │   (one lightweight Linux VM per      │        │
│  │    container — hardware isolation)   │        │
│  └──────────────────────────────────────┘        │
└─────────────────────────────────────────────────┘
```

### Key design decisions

1. **Shell out to `container` CLI** — Do NOT use CGo or Swift interop. Apple's `container` binary is the stable, well-maintained interface. We call it via `os/exec`. This keeps gocker pure Go, single binary, zero dependencies beyond `container` being installed.

2. **Docker REST API daemon** — Listen on a Unix socket (`~/.gocker/gocker.sock`). Implement the subset of Docker Engine API that lazydocker, Portainer, Testcontainers, VS Code Docker extension, and similar tools actually use. Set `DOCKER_HOST=unix://$HOME/.gocker/gocker.sock` and everything just works.

3. **urfave/cli/v3** for CLI framework — Not Cobra. Lighter, more Go-idiomatic. **Note: v3 API differs from v2** — root is `*cli.Command` not `*cli.App`, action signature is `func(ctx context.Context, cmd *cli.Command) error`, flags use generics. See https://cli.urfave.org/ for v3 docs.

4. **koanf** for configuration — Config file at `~/.gocker/config.json`. Used for default network policies, agent templates, registry auth forwarding.

5. **Sandbox as first-class citizen** — `gocker sandbox` is not an afterthought. It's the differentiating feature. AI agent sandboxing with per-container microVM isolation, network policy enforcement, workspace mounting, and agent lifecycle management.

---

## Project Structure

```
gocker/
├── main.go                     # Entry point, wires up urfave app
├── go.mod
├── go.sum
├── CLAUDE.md                   # This file (copy of plan)
├── README.md
├── LICENSE                     # Apache 2.0
├── Makefile                    # build, install, test, lint
├── .goreleaser.yml             # For releases + Homebrew
│
├── cmd/                        # CLI command definitions
│   ├── root.go                 # Top-level urfave app definition
│   ├── run.go                  # gocker run
│   ├── ps.go                   # gocker ps
│   ├── stop.go                 # gocker stop
│   ├── rm.go                   # gocker rm
│   ├── exec.go                 # gocker exec
│   ├── logs.go                 # gocker logs
│   ├── build.go                # gocker build
│   ├── pull.go                 # gocker pull
│   ├── push.go                 # gocker push
│   ├── images.go               # gocker images
│   ├── rmi.go                  # gocker rmi
│   ├── inspect.go              # gocker inspect
│   ├── network.go              # gocker network create/ls/rm/connect/disconnect
│   ├── volume.go               # gocker volume create/ls/rm/inspect
│   ├── system.go               # gocker system info/prune
│   ├── compose.go              # gocker compose up/down/ps/logs/restart
│   ├── daemon.go               # gocker daemon start/stop/status
│   └── sandbox/
│       ├── sandbox.go          # gocker sandbox (parent command)
│       ├── run.go              # gocker sandbox run <agent> <workspace>
│       ├── ls.go               # gocker sandbox ls
│       ├── stop.go             # gocker sandbox stop <name>
│       ├── rm.go               # gocker sandbox rm <name>
│       ├── attach.go           # gocker sandbox attach <name>
│       ├── logs.go             # gocker sandbox logs <name>
│       └── network.go          # gocker sandbox network --policy/--allow-host
│
├── engine/                     # Core engine — translates operations to `container` CLI
│   ├── engine.go               # Engine struct, constructor, shared exec helpers
│   ├── container.go            # Container lifecycle: run, stop, rm, start, inspect, exec, logs
│   ├── image.go                # Image operations: pull, push, list, remove, tag, build
│   ├── network.go              # Network operations: create, ls, rm, connect, disconnect
│   ├── volume.go               # Volume operations: create, ls, rm, inspect
│   ├── exec.go                 # Helpers for calling `container` CLI + parsing output
│   └── types.go                # Internal types: ContainerInfo, ImageInfo, NetworkInfo, etc.
│
├── api/                        # Docker-compatible REST API server
│   ├── server.go               # HTTP server on Unix socket, router setup
│   ├── middleware.go            # Logging, API version handling
│   ├── containers.go           # /containers/* handlers
│   ├── images.go               # /images/* handlers
│   ├── networks.go             # /networks/* handlers
│   ├── volumes.go              # /volumes/* handlers
│   ├── system.go               # /_ping, /version, /info handlers
│   └── types.go                # Docker API response types (import from docker/docker or define inline)
│
├── sandbox/                    # Sandbox management logic
│   ├── manager.go              # SandboxManager: create, list, stop, rm, attach
│   ├── state.go                # Persisted state at ~/.gocker/sandboxes/<name>.json
│   ├── template.go             # Agent template definitions (Claude, Codex, Gemini, etc.)
│   ├── network_policy.go       # pf firewall rules per container IP
│   ├── configsync.go           # Host agent config detection + mount generation
│   └── claudemd.go             # CLAUDE.md generator for sandbox context
│
├── compose/                    # Compose orchestration
│   ├── parser.go               # Parse docker-compose.yml (use compose-go library)
│   ├── orchestrator.go         # Start/stop services in dependency order
│   └── project.go              # Project state management
│
├── config/                     # Configuration
│   └── config.go               # koanf-based config loading from ~/.gocker/config.json
│
├── format/                     # Output formatting
│   ├── table.go                # Docker-style table output (text/tabwriter)
│   └── json.go                 # JSON output for --format json
│
└── testutil/                   # Test helpers
    ├── mock_exec.go            # Mock exec.Command for unit tests
    └── fixtures/               # Test fixture files
```

---

## Implementation Plan — Build Order

### Phase 1: Foundation + Core Container Ops

**Goal**: `gocker run -it alpine sh` works, `gocker ps` shows it.

#### Step 1.1: Project bootstrap

- `go mod init github.com/lunguini/gocker`
- Add dependencies: `github.com/urfave/cli/v3`
- Create `main.go` with top-level urfave app
- Create `cmd/root.go` with app definition, global flags (`--format`, `--debug`)
- Create `Makefile` with targets: `build`, `install` (`/usr/local/bin/gocker`), `test`, `lint`
- Verify: `go build && ./gocker --help` shows help

#### Step 1.2: Engine — exec helpers

- Create `engine/engine.go`:
  - `Engine` struct holding config (path to `container` binary, defaults to `/usr/local/bin/container`)
  - `func (e *Engine) Exec(ctx context.Context, args ...string) (stdout []byte, stderr []byte, err error)` — runs `container` with args, captures output
  - `func (e *Engine) ExecInteractive(ctx context.Context, args ...string) error` — runs with stdin/stdout/stderr attached (for `-it` mode)
  - `func (e *Engine) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error)` — for streaming logs

- Create `engine/types.go`:
  - `ContainerInfo` struct: ID, Name, Image, State, IP, Ports, Created, Command
  - `ImageInfo` struct: Name, Tag, Digest, Size, Created, Arch
  - Helper functions to parse `container ls` and `container images ls` output

#### Step 1.3: Core container commands

Implement these CLI commands in `cmd/`, each delegating to `engine/container.go`:

- **`gocker run`**: Maps to `container run`. Key flags to support:
  - `-it` → `container run -it`
  - `-d` / `--detach` → `container run -d`
  - `--name` → `container run --name`
  - `-v` / `--volume` → `container run -v`
  - `-p` / `--publish` → `container run --publish` (note: Apple Container uses dedicated IPs, port mapping may differ)
  - `-e` / `--env` → `container run -e`
  - `--env-file` → read file, pass as multiple `-e`
  - `-w` / `--workdir` → `container run -w`
  - `--rm` → `container run --rm`
  - `--network` → `container run --network`
  - `--platform` → `container run --platform` (pass through directly, Apple Container accepts `linux/amd64` format)
  - `--cpus` / `-c` → `container run --cpus` (Apple Container default: 4 CPUs per container)
  - `--memory` / `-m` → `container run --memory` (Apple Container default: 1GB per container, supports K/M/G/T/P suffixes)

- **`gocker ps`**: Maps to `container ls`. Parse output, reformat as Docker-style table:
  ```
  CONTAINER ID   IMAGE          COMMAND   CREATED        STATUS    PORTS     NAMES
  a1b2c3d4e5f6   alpine:latest  sh        2 minutes ago  running   -         my-container
  ```
  - Support `--all` / `-a` → `container ls -a`
  - Support `--quiet` / `-q` → print only IDs
  - Support `--format json`

- **`gocker stop`**: Maps to `container stop <name|id>`

- **`gocker rm`**: Maps to `container delete <name|id>` (or alias `container rm`). Support `-f` / `--force` (stop then rm).

- **`gocker start`**: Maps to `container start <name|id>`

- **`gocker exec`**: Maps to `container exec`. Support `-it` for interactive.

- **`gocker logs`**: Maps to `container logs <name|id>`. Support `-f` / `--follow`.

- **`gocker inspect`**: Maps to `container inspect <name|id>`. Return Docker-compatible JSON (wrap in array).

- **`gocker pull`**: Maps to `container image pull <image>`.

- **`gocker images`**: Maps to `container image list`. Apple supports `--format json` natively. Reformat JSON output to Docker-style table or pass through.

- **`gocker rmi`**: Maps to `container image delete <image>`.

- **`gocker build`**: Maps to `container build`. Support `-t` / `--tag`, `-f` / `--file`, build context path.

- **`gocker push`**: Maps to `container image push <image>`.

#### Step 1.4: Output formatting

- Create `format/table.go`: Docker-style table formatter using `text/tabwriter`
- Create `format/json.go`: JSON output with proper Docker field names
- All commands support `--format json` flag for machine-readable output

### Phase 2: Network + Volume Management

#### Step 2.1: Network commands

- **`gocker network create <name>`** → `container network create <name>`
- **`gocker network ls`** → `container network ls`
- **`gocker network rm <name>`** → `container network rm <name>`
- **`gocker network connect <network> <container>`** → `container network connect`
- **`gocker network disconnect <network> <container>`** → `container network disconnect`
- **`gocker network inspect <name>`** → `container network inspect` (return Docker-compatible JSON)

#### Step 2.2: Volume commands

- **`gocker volume create <name>`** → `container volume create <name>`
- **`gocker volume ls`** → `container volume ls`
- **`gocker volume rm <name>`** → `container volume rm <name>`
- **`gocker volume inspect <name>`** → `container volume inspect` (return Docker-compatible JSON)

### Phase 3: Docker REST API Daemon

**Goal**: `DOCKER_HOST=unix://$HOME/.gocker/gocker.sock lazydocker` works.

#### Step 3.1: Daemon lifecycle

- **`gocker daemon start`**: Start API server as background process. Write PID to `~/.gocker/daemon.pid`. Create Unix socket at `~/.gocker/gocker.sock`.
- **`gocker daemon stop`**: Read PID, send SIGTERM.
- **`gocker daemon status`**: Check if PID is alive, report socket path.
- Daemon should also be startable in foreground mode for debugging: `gocker daemon start --foreground`

#### Step 3.2: API server core

Create `api/server.go`:
- `net.Listen("unix", socketPath)` 
- `http.ServeMux` or a lightweight router (standard library is fine)
- API versioning: accept `/v1.41/containers/json` and `/containers/json` (strip version prefix)
- All handlers take `engine.Engine` and translate HTTP requests to engine calls

#### Step 3.3: Implement Docker API endpoints

Each handler lives in its own file in `api/`. Response schemas MUST match Docker's API spec so clients parse them correctly. Import types from `github.com/docker/docker/api/types` or define compatible structs inline.

**Essential endpoints (ordered by priority):**

```
GET    /_ping                              → "OK" (required by almost every client)
GET    /version                            → {"Version": "gocker-0.1.0", "ApiVersion": "1.41", ...}
GET    /info                               → System info (OS, arch, container count, etc.)

GET    /containers/json                    → List containers (gocker ps)
POST   /containers/create                  → Create container (returns ID)
POST   /containers/{id}/start             → Start container
POST   /containers/{id}/stop              → Stop container
POST   /containers/{id}/kill              → Kill container
DELETE /containers/{id}                    → Remove container
GET    /containers/{id}/json              → Inspect container
GET    /containers/{id}/logs              → Container logs (support streaming via chunked transfer)
POST   /containers/{id}/exec              → Create exec instance
POST   /exec/{id}/start                   → Start exec (this is a 2-step process in Docker API)

GET    /images/json                        → List images
POST   /images/create                      → Pull image (action=pull, fromImage query param)
DELETE /images/{name}                      → Remove image
GET    /images/{name}/json                → Inspect image

GET    /networks                           → List networks
GET    /networks/{id}                      → Inspect network
POST   /networks/create                    → Create network
DELETE /networks/{id}                      → Remove network
POST   /networks/{id}/connect             → Connect container to network
POST   /networks/{id}/disconnect          → Disconnect container from network

GET    /volumes                            → List volumes
POST   /volumes/create                     → Create volume
DELETE /volumes/{name}                     → Remove volume
GET    /volumes/{name}                     → Inspect volume
```

**Response format requirements for client compatibility:**

- `GET /containers/json` must return `[]ContainerJSON` with fields: `Id`, `Names` (array with leading `/`), `Image`, `State`, `Status`, `Created` (unix timestamp), `Ports`, `NetworkSettings`
- `GET /containers/{id}/json` must return full `ContainerJSON` (lazydocker reads `Config.Env`, `HostConfig.Binds`, `NetworkSettings.Networks`, `State.Status`, `State.Running`, `State.StartedAt`)
- `POST /containers/create` accepts JSON body with `Image`, `Cmd`, `Env`, `HostConfig.Binds`, `HostConfig.PortBindings`, `HostConfig.NetworkMode`, returns `{"Id": "...", "Warnings": []}`
- `GET /images/json` must return array with `Id`, `RepoTags`, `Created`, `Size`

#### Step 3.4: Exec handling

Docker's exec API is two-phase:
1. `POST /containers/{id}/exec` — create exec, returns exec ID
2. `POST /exec/{id}/start` — start exec, optionally hijacks connection for interactive

For simplicity, store exec configs in memory (map[string]ExecConfig), and on start, run `container exec`. Interactive exec with TTY requires WebSocket or connection hijacking — implement non-interactive first, add interactive as a follow-up.

### Phase 4: Sandbox

**Goal**: `gocker sandbox run claude ~/my-project` gives you an isolated Claude Code environment.

#### Step 4.1: Sandbox state management

Create `sandbox/state.go`:
- State file per sandbox at `~/.gocker/sandboxes/<name>.json`
- Tracks: name, agent, workspace path, container ID, status (running/stopped), created timestamp, network policy, allowed hosts, container IP

Create `sandbox/manager.go`:
- `Create(name, agent, workspace, opts)` — pull template, create container, save state
- `Start(name)` — start existing stopped sandbox
- `Stop(name)` — stop sandbox container
- `Remove(name)` — stop + rm container + delete state file
- `List()` — read all state files
- `Attach(name)` — reattach to running sandbox's TTY
- `GetLogs(name, follow)` — get sandbox logs

#### Step 4.2: Agent templates

Create `sandbox/template.go`:

Templates are OCI images with agent tooling pre-installed. Define template specs:

```go
type AgentTemplate struct {
    Name        string   // "claude", "codex", "gemini"
    Image       string   // OCI image reference
    EntryCmd    []string // Default entrypoint command
    EnvVars     []string // Required env vars (e.g., "ANTHROPIC_API_KEY")
    DefaultArgs []string // e.g., ["--dangerously-skip-permissions"]
}
```

Built-in templates:

- **claude**: Image based on `ubuntu:24.04` + Node.js + `@anthropic-ai/claude-code`. Entry: `claude --dangerously-skip-permissions`. Requires: `ANTHROPIC_API_KEY`.
- **codex**: Image based on `ubuntu:24.04` + Node.js + `@openai/codex`. Entry: `codex --full-auto`. Requires: `OPENAI_API_KEY`.
- **custom**: User specifies image + entry command via `--image` and `--entrypoint` flags.

For MVP, ship a Dockerfile per template in the repo under `templates/` and build on first use (cache the image locally). Later, publish pre-built images to GHCR.

#### Step 4.3: Sandbox run command

`gocker sandbox run <agent> [workspace] [flags]`

Flags:
- `--name` — custom sandbox name (default: `<agent>-<dirname>`)
- `--network-policy` — `allow` (default) or `deny`
- `--allow-host` — repeatable, allowed domains when policy is `deny`
- `--env` / `-e` — additional env vars
- `--image` — override template image
- `--entrypoint` — override template entry command
- `--detach` / `-d` — run in background
- `--sync-config` / `--no-sync-config` — sync host agent config (user settings, global CLAUDE.md) into sandbox. **Default: on.** Use `--no-sync-config` for a clean-slate environment. Note: managed settings still mount unless `--no-managed-settings` is also passed.
- `--sync-state` / `--no-sync-state` — sync `~/.claude.json` (OAuth session, preferences, MCP configs, per-project trust/allowed-tools, caches) into sandbox with write access. **Default: on.** This is what makes auth "just work" without re-login.
- `--no-managed-settings` — skip mounting enterprise managed settings. Only for testing outside enterprise policy.

Behavior:
1. Check if sandbox `<n>` already exists in state. If running, reattach. If stopped, start and reattach.
2. If new: build/pull template image if not cached.
3. Detect host agent config and generate volume mounts (see Step 4.6).
4. Create the container via engine:
   ```
   container run -it --name <sandbox-name> \
     -v <workspace>:/workspace \
     -w /workspace \
     -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
     # Config sync (when --sync-config, the default):
     -v ~/.claude/settings.json:/root/.claude/settings.json:ro \
     -v ~/.claude.md:/root/.claude.md:ro \
     # State sync (when --sync-state, the default):
     -v ~/.claude.json:/root/.claude.json \
     # Managed settings (always unless --no-managed-settings):
     -v "/Library/Application Support/ClaudeCode/managed-settings.json":/etc/claude-code/managed-settings.json:ro \
     -v "/Library/Application Support/ClaudeCode/managed-mcp.json":/etc/claude-code/managed-mcp.json:ro \
     <template-image> \
     <entry-command>
   ```
5. After container starts, get its IP via `container inspect`.
6. Apply network policy if `--network-policy deny` (see Step 4.4).
7. Save state to `~/.gocker/sandboxes/<n>.json`.
8. If not detached, attach to container TTY.

#### Step 4.4: Network policy enforcement

Apple Container assigns each container its own IP address. Use macOS `pf` (packet filter) to control egress:

```go
// In sandbox/network_policy.go

func ApplyDenyPolicy(containerIP string, allowedHosts []string) error {
    // 1. Resolve allowed hosts to IPs
    // 2. Write pf anchor rules:
    //    block drop out quick on * from <containerIP> to any
    //    pass out quick on * from <containerIP> to <allowed-ip-1>
    //    pass out quick on * from <containerIP> to <allowed-ip-2>
    //    ... (always allow DNS: port 53)
    // 3. Load anchor: pfctl -a gocker/<sandbox-name> -f <rules-file>
    // 4. Enable pf if not already: pfctl -e
}

func RemovePolicy(sandboxName string) error {
    // pfctl -a gocker/<sandbox-name> -F all
}
```

Alternatively (simpler for MVP): run a lightweight HTTP/HTTPS proxy sidecar in a second container on the same network. Route the sandbox container's traffic through it. The proxy has the allow/deny list. This avoids needing root for pf but adds a second container per sandbox.

**Decision: start with pf approach.** It's cleaner (one container per sandbox) and Apple Container's per-container IP makes it natural. Require `sudo` only for the `pfctl` calls; the rest of gocker runs unprivileged.

#### Step 4.5: CLAUDE.md generation

Create `sandbox/claudemd.go`:

When creating a Claude sandbox, generate a `CLAUDE.md` file at the workspace root (if not already present) with:
- OS and arch info from inside the container
- Available tools (detected: git, node, python, go, etc.)
- Workspace path: `/workspace`
- Network policy summary: what's allowed/blocked
- Agent-specific tips (e.g., "You are running in an isolated Apple Container microVM. You have full permissions. Your workspace is at /workspace.")

This file is generated ONCE on sandbox creation and not overwritten if it already exists.

#### Step 4.6: Host config sync

Create `sandbox/configsync.go`:

This makes gocker sandbox actually usable for daily work. Without it, every sandbox is a blank-slate Claude Code install — no permissions, no allowed tools, no MCP servers, no enterprise policies. You have to reconfigure everything every time.

**Claude Code config paths (from https://code.claude.com/docs/en/settings):**

Claude Code uses a hierarchical settings system with precedence: Managed (highest) → CLI args → Local → Project → User (lowest).

The key files:

| Host path (macOS) | Container path (Linux) | Mode | What it contains |
|---|---|---|---|
| `~/.claude/settings.json` | `/root/.claude/settings.json` | ro | User settings: permission rules (allow/deny), env vars, tool config, hooks, sandbox settings. Applies to all projects. |
| `~/.claude.json` | `/root/.claude.json` | rw | The big state file: OAuth session, preferences (theme, notifications, editor mode), MCP server configs (local-scoped), per-project state (allowed tools, trust settings), caches. Needs rw for token refresh and state updates. |
| `~/.claude.md` | `/root/.claude.md` | ro | Global user-level CLAUDE.md memory file loaded at startup. |
| `/Library/Application Support/ClaudeCode/managed-settings.json` | `/etc/claude-code/managed-settings.json` | ro | Enterprise/IT-deployed managed settings. Highest precedence — cannot be overridden by user or project settings. Contains enforced permission deny lists, `disableBypassPermissionsMode`, `strictKnownMarketplaces`, etc. |
| `/Library/Application Support/ClaudeCode/managed-mcp.json` | `/etc/claude-code/managed-mcp.json` | ro | Enterprise-managed MCP servers. When present, users cannot add MCP servers through `claude mcp add` or config files — only managed servers load. |
| `.claude/settings.json` (in workspace) | Already in workspace mount | rw | Project settings shared with team. |
| `.claude/*.local.*` (in workspace) | Already in workspace mount | rw | Local project overrides (gitignored). |
| `.mcp.json` (in workspace) | Already in workspace mount | rw | Project-scoped MCP servers. |

**Key design decisions:**

- **`~/.claude.json` is rw** — This is the most important mount. It holds the OAuth session (so the agent doesn't need to re-authenticate), MCP server configs, and per-project trust state. Without write access, token refresh fails during long sessions and state updates are lost.
- **`~/.claude/settings.json` is ro** — The agent can USE your permission rules but can't modify them from inside the sandbox. Your allowed-tools, deny lists, and hooks are enforced but protected.
- **Managed settings are ALWAYS ro** — Non-negotiable. These are enterprise security policies. If the file exists on the host, mount it. The path translation (macOS → Linux) is necessary because the host is macOS but the container runs Linux.
- **Managed settings survive `--no-sync-config`** — Even if you want a clean-slate user config, enterprise policies should still apply unless you explicitly pass `--no-managed-settings`. This is a deliberate security decision.
- **Server-managed settings** (fetched from Anthropic servers) are cached inside `~/.claude.json`. By mounting that file, cached server-managed settings carry over automatically.
- **Project-level config needs no special handling** — `.claude/` and `.mcp.json` are inside the workspace directory, which is already mounted at `/workspace`. They're available rw by default.
- **All config mounts are optional** — If a file doesn't exist on the host, skip it silently. A fresh user who hasn't customized anything shouldn't get errors.

**Implementation:**

```go
type ConfigMount struct {
    HostPath      string // Absolute path on macOS host
    ContainerPath string // Path inside Linux container
    ReadOnly      bool
    Optional      bool   // true = skip silently if not found on host
}

// Claude Code specific config discovery
func ClaudeConfigMounts(syncConfig, syncState, managedSettings bool) []ConfigMount {
    home, _ := os.UserHomeDir()
    var mounts []ConfigMount

    if syncConfig {
        mounts = append(mounts,
            ConfigMount{filepath.Join(home, ".claude", "settings.json"), "/root/.claude/settings.json", true, true},
            ConfigMount{filepath.Join(home, ".claude.md"), "/root/.claude.md", true, true},
        )
    }

    if syncState {
        mounts = append(mounts,
            ConfigMount{filepath.Join(home, ".claude.json"), "/root/.claude.json", false, true},
        )
    }

    if managedSettings {
        mounts = append(mounts,
            ConfigMount{"/Library/Application Support/ClaudeCode/managed-settings.json", "/etc/claude-code/managed-settings.json", true, true},
            ConfigMount{"/Library/Application Support/ClaudeCode/managed-mcp.json", "/etc/claude-code/managed-mcp.json", true, true},
        )
    }

    return mounts
}

// GenerateMountFlags checks which paths exist and returns -v flags
func GenerateMountFlags(mounts []ConfigMount) []string {
    var flags []string
    for _, m := range mounts {
        if m.Optional {
            if _, err := os.Stat(m.HostPath); os.IsNotExist(err) {
                continue // skip missing optional mounts
            }
        }
        flag := fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath)
        if m.ReadOnly {
            flag += ":ro"
        }
        flags = append(flags, "-v", flag)
    }
    return flags
}
```

**Extensible for other agents:**

```go
var agentConfigFuncs = map[string]func(syncConfig, syncState, managedSettings bool) []ConfigMount{
    "claude": ClaudeConfigMounts,
    "codex":  CodexConfigMounts,  // implement when needed
}
```

### Phase 5: Compose

**Goal**: `gocker compose up -d` with a standard `docker-compose.yml` works.

#### Step 5.1: Compose file parsing

Use `github.com/compose-spec/compose-go/v2` (the official Docker Compose file parser, Apache 2.0):
- Parse `docker-compose.yml` or `compose.yml`
- Resolve environment variables and `.env` files
- Resolve `depends_on` ordering

#### Step 5.2: Compose orchestrator

Create `compose/orchestrator.go`:

- **`up`**: For each service in dependency order:
  1. Create network if specified
  2. Create volume if specified
  3. Pull image if not present
  4. Build image if `build` context specified
  5. Run container with mapped ports, volumes, env, network
  6. Track service→container mapping in `~/.gocker/compose/<project>/state.json`

- **`down`**: Stop and remove all containers for the project. Remove project network.
- **`ps`**: List containers for the project.
- **`logs`**: Aggregate logs from all services (or a specific service).
- **`restart`**: Stop + start a specific service.

#### Step 5.3: Compose CLI

`gocker compose` subcommands:
- `gocker compose up [-d] [-f <file>]` 
- `gocker compose down [-f <file>]`
- `gocker compose ps [-f <file>]`
- `gocker compose logs [service] [-f <file>] [--follow]`
- `gocker compose restart [service] [-f <file>]`

### Phase 6: Polish + Distribution

#### Step 6.1: System commands

- `gocker system info` — Show gocker version, `container` CLI version, socket path, config path, number of containers/images
- `gocker system prune` — Remove stopped containers and unused images

#### Step 6.2: Configuration

Create `config/config.go` using koanf:

Config file at `~/.gocker/config.json`:
```json
{
  "containerBinary": "/usr/local/bin/container",
  "socketPath": "~/.gocker/gocker.sock",
  "sandbox": {
    "defaultNetworkPolicy": "allow",
    "defaultAllowedHosts": [
      "api.anthropic.com",
      "api.openai.com",
      "registry.npmjs.org",
      "github.com"
    ]
  },
  "templates": {
    "claude": {
      "image": "ghcr.io/lunguini/gocker-claude:latest"
    }
  }
}
```

#### Step 6.3: Shell completions

urfave/cli/v3 supports shell completions. Enable for bash, zsh, fish:
- `gocker --generate-completion bash`
- `gocker --generate-completion zsh`

#### Step 6.4: Distribution

- **Makefile targets**: `build`, `install`, `test`, `lint`
- **GoReleaser** config for cross-compilation and Homebrew tap:
  ```
  brew tap lunguini/tap
  brew install gocker
  ```
- **GitHub Actions** workflow: test on push, release on tag

---

## Flag Mapping Reference

Key differences between Docker CLI and Apple `container` CLI flags:

| Docker flag | Apple `container` equivalent | Notes |
|---|---|---|
| `docker run -d` | `container run -d` | Same |
| `docker run -it` | `container run -it` | Same |
| `docker run --name X` | `container run --name X` | Same |
| `docker run -v host:guest` | `container run -v host:guest` | Same. Also supports `--mount source=X,target=Y` syntax and `--volume` long form |
| `docker run -p 8080:80` | `container run -p 8080:80` | Same. Both `-p` shorthand and `--publish` long form work. Each container gets its own IP, so port conflicts are less of an issue |
| `docker run -e KEY=VAL` | `container run -e KEY=VAL` | Same |
| `docker run --network X` | `container run --network X` | Same |
| `docker run --platform linux/amd64` | `container run --platform linux/amd64` | Same flag. Apple also supports `--arch amd64` and `--os linux` separately, but `--platform` works directly and takes precedence |
| `docker ps` | `container ls` | Different command name |
| `docker ps -a` | `container ls -a` | Same flag |
| `docker rm` | `container delete` (alias: `container rm`) | Apple uses `delete` as primary, `rm` as alias |
| `docker images` | `container image list` (alias: `container images ls`) | Different command structure; supports `--format json` natively |
| `docker rmi` | `container image delete` | Different command name (`delete` not `rm`) |
| `docker build -t X .` | `container build -t X .` | Same |
| `docker logs -f` | `container logs --follow` | Apple uses `--follow` long form (verify if `-f` shorthand exists in current version) |
| `docker inspect` | `container inspect` | Same, but output format may differ |

**IMPORTANT**: Apple Container supports `--format json` natively on most commands (e.g., `container list --format json`, `container image list --format json`, `container inspect`). The JSON schema differs from Docker's. The engine layer MUST parse Apple Container's output and reformat to Docker-compatible structures. Always test actual `container` CLI output and write parsers accordingly. Do not assume the format matches Docker.

---

## Testing Strategy

### Unit tests

- Mock `exec.Command` in engine tests — inject a mock executor that returns canned output
- Test flag mapping: ensure Docker flags translate to correct `container` CLI args
- Test output parsing: ensure `container ls` output parses into correct `ContainerInfo` structs
- Test API response formatting: ensure JSON matches Docker API spec

### Integration tests (require macOS 26 + Apple Container)

- Tag with `//go:build integration`
- Actually run `container` CLI and verify gocker's output matches
- Test sandbox lifecycle: create → attach → stop → rm
- Test compose: up → verify services running → down

### Test fixtures

- Store sample `container ls` output, `container inspect` output, etc. in `testutil/fixtures/`
- Use for unit test parsing validation

---

## Dependencies

```
github.com/urfave/cli/v3          — CLI framework
github.com/knadh/koanf/v2         — Configuration
github.com/compose-spec/compose-go/v2  — Compose file parsing
github.com/docker/docker           — Docker API response types (import from api/types/container, api/types, etc. — types only, not the engine). Note: this is a heavy dependency; alternatively define compatible structs inline to keep the binary small.
```

No CGo. No Swift. Single binary. `go build` and done.

---

## Prerequisites

The user must have installed on their Mac:
- macOS 26 (Tahoe) on Apple Silicon
- Apple `container` CLI (`/usr/local/bin/container`) — installed from https://github.com/apple/container/releases
- `container system start` must have been run at least once
- Go 1.22+ for building gocker

---

## Build & Run

```bash
# Build
make build
# or: go build -o gocker .

# Install to /usr/local/bin
make install
# or: sudo cp gocker /usr/local/bin/

# Run
gocker --help
gocker run -it alpine:latest sh
gocker ps

# Start daemon for Docker API compatibility
gocker daemon start
export DOCKER_HOST=unix://$HOME/.gocker/gocker.sock
lazydocker  # just works

# Sandbox
export ANTHROPIC_API_KEY=sk-ant-...
gocker sandbox run claude ~/my-project
gocker sandbox ls
gocker sandbox attach claude-my-project
gocker sandbox stop claude-my-project
gocker sandbox rm claude-my-project
```

---

## Success Criteria

### MVP (v0.1.0)
- [ ] `gocker run`, `ps`, `stop`, `rm`, `exec`, `logs`, `inspect` work
- [ ] `gocker pull`, `images`, `rmi`, `build`, `push` work
- [ ] `gocker daemon start` exposes Docker-compatible API on Unix socket
- [ ] `lazydocker` works with `DOCKER_HOST` pointing to gocker's socket
- [ ] `gocker sandbox run claude ~/project` creates isolated sandbox
- [ ] Host Claude Code config (settings, auth, managed settings) synced into sandbox by default
- [ ] `gocker sandbox ls/stop/rm/attach` work
- [ ] Network policy enforcement (deny all + allowlist) works

### v0.2.0
- [ ] `gocker compose up/down/ps/logs` with standard docker-compose.yml
- [ ] `gocker network` and `gocker volume` commands
- [ ] Portainer compatibility
- [ ] Testcontainers compatibility
- [ ] Shell completions
- [ ] Homebrew formula

### v0.3.0
- [ ] Pre-built agent template images on GHCR
- [ ] `gocker sandbox run codex`, `gocker sandbox run gemini`
- [ ] CLAUDE.md auto-generation
- [ ] Config file support via koanf
- [ ] GoReleaser + GitHub Actions CI/CD
