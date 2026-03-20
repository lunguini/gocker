# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Gocker

Docker-compatible CLI and REST API daemon that wraps Apple's `container` CLI on macOS 26+. Each container is a lightweight Linux microVM via `Virtualization.framework`. The killer feature is `gocker sandbox` — AI agent sandboxing with hardware-level isolation.

## Commands

```bash
make build     # go build -ldflags "-X main.version=0.1.0" -o gocker .
make install   # build + sudo cp to /usr/local/bin/
make test      # go test ./...
make lint      # golangci-lint run ./...
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

- **`main.go`** — entry point, calls `cmd.NewApp().Run()`
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

## Command Mapping (non-obvious)

- `docker rm` → `container delete` (not `container rm`)
- `docker rmi` → `container image delete`
- `docker kill` is not separate — it calls `ContainerStop`

## Build Plan

See `GOCKER_PLAN.md` for the full phased implementation plan and design rationale.
