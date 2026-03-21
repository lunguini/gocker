<p align="center">
    <img src="./assets/header.jpg" alt="gocker logo" width="360">
</p>

# gocker

Docker-compatible CLI and API daemon for [Apple Container](https://github.com/apple/container) on macOS 26+.

Every container runs as a lightweight Linux microVM backed by Apple's `Virtualization.framework` — hardware-level isolation, not just namespaces.

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

## License

Apache 2.0
