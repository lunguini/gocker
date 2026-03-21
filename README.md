<p align="center">
    <img src="./assets/header.jpg" alt="gocker logo" width="360">
</p>

# gocker

Docker-compatible CLI and API daemon for [Apple Container](https://github.com/apple/container) on macOS 26+.

Every container runs as a lightweight Linux microVM backed by Apple's `Virtualization.framework` — hardware-level isolation, not just namespaces.

## Why gocker?

| | Docker Desktop | gocker |
|---|---|---|
| **How it works** | Runs a hidden Linux VM, then runs containers inside it | Each container *is* its own lightweight VM — native on Apple Silicon |
| **Setup** | Download installer, sign in, allocate resources | `gocker setup` — one command, done |
| **CLI** | `docker run`, `docker ps`, ... | Same commands — just swap `docker` for `gocker` |
| **AI sandboxing** | `docker sandbox` — requires Docker Desktop | `gocker sandbox run claude ./` — native, no Docker needed |
| **Overhead** | Docker Desktop daemon, ~2GB RAM idle | Single static binary, no background daemon required |
| **Isolation** | Process-level (namespaces/cgroups) | Hardware-level (Apple Virtualization.framework) |

## Requirements

- macOS 26+ (Tahoe) on Apple Silicon
- Apple's `container` CLI

## Install

```bash
go install github.com/lunguini/gocker@latest
```

## Getting Started

```bash
# Check prerequisites and install Apple Container if needed
gocker setup

# Run a container
gocker run ubuntu:latest echo "hello from a microVM"

# Run an interactive container
gocker run -it ubuntu:latest /bin/bash
```

## Usage

gocker mirrors Docker's CLI interface:

```bash
gocker run -d --name web -p 8080:80 nginx     # Run in background
gocker ps                                       # List containers
gocker logs web                                 # View logs
gocker exec -it web /bin/sh                     # Exec into container
gocker stop web                                 # Stop container
gocker rm web                                   # Remove container
```

Images, networks, and volumes work the same way:

```bash
gocker pull ubuntu:latest
gocker images
gocker network create mynet
gocker volume create mydata
```

Use `--format json` on any command for JSON output.

## AI Agent Sandboxing

The killer feature. Run AI agents in hardware-isolated microVMs with host configs synced automatically:

```bash
# Run Claude Code in a sandbox (mounts current dir as /workspace)
gocker sandbox run claude ./

# Run with a custom name
gocker sandbox run claude ./ --name my-project

# Run in background
gocker sandbox run claude ./ -d

# Manage sandboxes
gocker sandbox ls
gocker sandbox attach my-project
gocker sandbox logs my-project
gocker sandbox stop my-project
gocker sandbox rm my-project
```

Sandboxes automatically:
- Mount your workspace into the VM
- Sync host Claude settings (plugins, marketplaces) with sandbox-safe defaults
- Forward `ANTHROPIC_API_KEY` from your environment
- Allocate 4GB memory for Claude Code

## Docker API Compatibility

gocker includes a Docker-compatible REST API daemon that lets existing Docker tools work out of the box:

```bash
# Start the API daemon
gocker daemon start

# Now tools like lazydocker, Portainer, and Testcontainers
# can connect via ~/.gocker/gocker.sock
```

## Building

```bash
make build     # Build the binary
make install   # Build and install to /usr/local/bin
make test      # Run tests
make lint      # Run linter
```

## Roadmap

- [x] Core container commands (`run`, `ps`, `stop`, `rm`, `exec`, `logs`, `inspect`, `start`)
- [x] Image management (`pull`, `push`, `images`, `rmi`, `build`)
- [x] Network management (`network create/ls/rm/connect/disconnect`)
- [x] Volume management (`volume create/ls/rm/inspect`)
- [x] Docker REST API daemon on Unix socket (`gocker daemon start`)
- [x] AI sandbox — `gocker sandbox run claude ./` with config sync
- [x] Auto-setup (`gocker setup` installs Apple Container CLI)
- [x] Template images published to Docker Hub
- [ ] Network policy enforcement (`--network-policy deny --allow-host api.anthropic.com`)
- [ ] `gocker compose up/down/ps/logs` with standard docker-compose.yml
- [ ] Codex and Gemini sandbox templates
- [ ] CLAUDE.md auto-generation for sandbox context
- [ ] Config file support (`~/.gocker/config.json`)
- [ ] Shell completions (bash, zsh, fish)
- [ ] Homebrew formula (`brew install gocker`)
- [ ] GoReleaser + GitHub Actions CI/CD

## License

Apache 2.0
