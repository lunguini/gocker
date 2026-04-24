# Shared VM internals

Notes on how the shared VM actually works — the parts that aren't obvious from reading the code and caused real bugs this session.

## Two gocker binaries, not one

In shared/hybrid mode there are **two** gocker processes, and they have different responsibilities:

- **Host gocker** — your Mac. Implements the CLI (`gocker run`, `gocker ps`) and the host API daemon (`gocker daemon start`, socket at `~/.gocker/gocker.sock`). Uses `SharedVMRuntime`, which *proxies* most operations into the VM via `container exec gocker-shared gocker <args>`.
- **In-VM gocker** — the Linux binary baked into `gocker-base`. Runs inside the shared VM. Implements the *real* work: calls `nerdctl` against the containerd daemon inside the VM. Also runs its own API daemon on `/var/run/docker.sock` inside the VM (started by `gocker-init.sh`).

A host-side fix doesn't reach the VM binary until a new image is published and the VM is recreated. See `image-channels.md` for the release channel story; see `sharedvm/manager.go` for the proxy machinery.

## Compose has two possible paths

`gocker compose` has been the source of repeated confusion. Two distinct code paths can handle `docker compose up -d`:

1. **gocker's compose proxy** (`cmd/compose.go` → `compose/proxy.go`) — used when you literally type `gocker compose ...` (or `docker compose ...` with `docker` shell-aliased). Shells out to `nerdctl compose` inside the VM.
2. **Real Docker Compose v2 against gocker's API daemon** — used when `docker` is a separate binary and `DOCKER_HOST` / docker context points at `~/.gocker/gocker.sock` (or at the in-VM `/var/run/docker.sock`). Compose makes standard Docker API calls against our daemon, which then proxies to nerdctl.

Path 1's output looks like `time=... level=info msg="Creating container ..."` (nerdctl's format).
Path 2's output looks like `[+] Running 4/4 ✘ service Error` (compose v2's format).

When diagnosing a compose failure, **check which path is actually being used** — the host daemon log (`~/.gocker/daemon.log`) will show traffic for path 2, nothing for path 1.

## CWD must be reachable inside the VM

`nerdctl compose` runs inside the VM and looks for the compose file at whatever `--project-directory` resolves to. Gocker's compose proxy translates the host CWD via VM mounts, but the translation only works if the CWD is under a mounted workspace.

Quirks that bit us:

- **Top-level `workspaceDirs:`** in `~/.gocker/config.yaml` used to be silently ignored because the struct wanted it under `sharedVM:`. Back-compat migration in `config.Load` lifts it now.
- **Symlink-resolved paths**: VM mounts are stored with symlinks resolved (`/tmp` → `/private/tmp` on macOS), so the proxy resolves the host CWD the same way before calling `TranslatePath`. Without this, `cd /tmp/foo && gocker compose up` silently failed.
- **Drift between manager and VM**: `Manager.mounts` used to be initialized from config and never reconciled with the running VM. If the VM had been created with fewer mounts, `TranslatePath` lied about coverage. `EnsureRunning` now calls `syncMountsFromVM` to sync the in-memory map with `container inspect`.
- **`listVMContainers` counts user workloads only**: it runs `nerdctl ps -q` inside the VM. An earlier implementation counted host-level Apple containers, which always included `gocker-shared` itself and any buildkit helpers, so `ExpandMounts` always refused to recreate the VM.

## Error surfacing: why "exit status 1" shows up

`NerdctlRuntime` historically wrapped shell-out errors as `fmt.Errorf("%s: %w", stderr, err)`. When stderr was empty — which happens when the daemon runs a command non-interactively and output is routed to `/dev/null` (daemon uses `os.StartProcess` with `Files: []*os.File{nil, nil, nil}` for detached mode), or when the process is killed — the error string degenerated to `": exit status 1"`.

Docker Compose v2 stringifies API 500 bodies as `Error response from daemon: <msg>`. So the user saw `Error response from daemon: exit status 1` and had no idea which service failed or why.

Fixes in place now (`engine/nerdctl.go`):

- `wrapRunErr` falls back stdout → command line when stderr is empty.
- `wrapNerdctlErr` gives a minimum "nerdctl produced no output" message for the same case.
- `ImagePull` and `Engine.ImagePull` use captured `Exec` when stdout isn't a TTY. Previously they used `ExecInteractive` unconditionally, which sent output to `os.Stderr` → `/dev/null` for the daemon.

When a compose-via-API call still produces `exit status 1` with no detail, the in-VM gocker binary is probably pre-fix. `gocker daemon vm update` to refresh.

## Fast diagnostic path

When compose fails with an opaque error, skip both daemons and run nerdctl directly inside the VM:

```
container exec gocker-shared nerdctl pull <image>
container exec gocker-shared nerdctl run --rm <image> <cmd>
container exec gocker-shared nerdctl compose -f /host/path/to/compose.yml up -d
```

The VM's nerdctl stderr shows the real error with no wrapping. If it works here but fails through the gocker API daemon, the bug is in gocker's translation/error-handling layer — not in the backend.

## Files and where state lives

| Path | Contents |
|---|---|
| `~/.gocker/config.yaml` | User config (host). |
| `~/.gocker/daemon.log` | Host API daemon request log (rolling). |
| `~/.gocker/daemon.pid` | Host daemon PID. |
| `~/.gocker/gocker.sock` | Host Docker API socket. |
| `~/.gocker/sharedvm/state.json` | Host-side record of VM status/mounts. |
| `~/.gocker/sandboxes/*.json` | Per-sandbox state. |
| `~/.gocker/compose/<project>/state.json` | Per-project compose state (full mode). |
| `/var/run/docker.sock` *(inside VM)* | Docker API served by the in-VM gocker daemon. |
| `/run/containerd/containerd.sock` *(inside VM)* | containerd socket nerdctl talks to. |
| `/root/.gocker/daemon.log` *(inside VM)* | In-VM daemon request log. |

When a fix doesn't appear to take effect, check both the host daemon state (is it restarted? `gocker daemon stop && gocker daemon start`) and the in-VM daemon (is the image up to date? `gocker daemon vm update`).
