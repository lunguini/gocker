package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newAICmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "ai",
		Usage: "Output AI-friendly CLI reference and workspace context",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			printAIContext(ctx, eng)
			return nil
		},
	}
}

func printAIContext(ctx context.Context, eng engine.Runtime) {
	fmt.Print(`# gocker CLI Reference (for AI agents)

gocker is a Docker-compatible CLI. Replace "docker" with "gocker" in most commands.

## Container lifecycle

gocker run [flags] IMAGE [COMMAND] [ARGS...]
  -d                    Run in background (detached)
  -it                   Interactive with TTY
  --name NAME           Container name
  -p HOST:CONTAINER     Publish port
  -v HOST:CONTAINER     Bind mount volume
  -e KEY=VALUE          Environment variable
  --env-file FILE       Load env vars from file
  -w DIR                Working directory
  -m SIZE               Memory limit (e.g., 4G)
  --rm                  Remove on exit
  --network NAME        Connect to network
  --restart POLICY      Restart policy (warning: not supported by Apple Container CLI)

gocker ps [-a]                         List containers (running, or all with -a)
gocker stop CONTAINER                  Stop a container
gocker start CONTAINER                 Start a stopped container
gocker rm [-f] CONTAINER               Remove a container
gocker exec [-it] CONTAINER CMD        Execute command in container
gocker logs [-f] CONTAINER             View container logs (-f to follow)
gocker inspect CONTAINER               Show container details (JSON)

## Images

gocker pull IMAGE                      Pull an image
gocker push IMAGE                      Push an image
gocker images                          List images
gocker rmi IMAGE                       Remove an image
gocker build [flags] PATH              Build an image

## Compose (docker-compose.yml compatible)

gocker compose up [-d] [-f FILE]       Start services (-d for background)
gocker compose down [-v] [-f FILE]     Stop and remove (-v removes volumes too)
gocker compose ps [-f FILE]            List service containers
gocker compose logs [SERVICE] [-f FILE]  View service logs
gocker compose restart [SERVICE]       Restart service(s)

## Networks

gocker network create NAME             Create a network
gocker network ls                      List networks
gocker network rm NAME                 Remove a network

## Volumes

gocker volume create NAME              Create a volume
gocker volume ls                       List volumes
gocker volume rm NAME                  Remove a volume

## AI Agent Sandboxing

gocker sandbox run AGENT [WORKSPACE]   Create and run agent sandbox
  AGENT: claude, codex, or custom
  WORKSPACE: directory to mount (default: current dir)
  -d                    Run in background
  --name NAME           Custom sandbox name
  --image IMAGE         Override template image
  -e KEY=VALUE          Extra environment variables

gocker sandbox ls                      List sandboxes
gocker sandbox attach NAME             Attach to running sandbox
gocker sandbox logs [-f] NAME          View sandbox logs
gocker sandbox stop NAME               Stop a sandbox
gocker sandbox rm NAME                 Remove a sandbox

## Daemon & API

gocker daemon start                    Start Docker-compatible API daemon
gocker daemon stop                     Stop the daemon
gocker daemon status                   Show daemon status
gocker daemon vm status                Shared VM status (hybrid/shared mode)
gocker daemon vm stop                  Stop the shared VM

API socket: ~/.gocker/gocker.sock
Use with Docker tools: DOCKER_HOST=unix://$HOME/.gocker/gocker.sock

## Configuration (~/.gocker/config.yaml)

isolation: full|hybrid|shared          VM isolation mode
sharedVM:
  image: docker.io/adyjay/gocker:base-latest
  memory: 4G
  cpus: 4

## Global flags

--format json                          JSON output on any command
--isolation MODE                       Override isolation mode
--debug                                Debug output

## Common patterns

# Run a web stack
gocker compose up -d

# Run Claude in a sandbox
gocker sandbox run claude ./

# Portainer
gocker daemon start
gocker volume create portainer_data
gocker run -d -p 9443:9443 --name portainer -v ~/.gocker/gocker.sock:/var/run/docker.sock -v portainer_data:/data portainer/portainer-ce:sts

# PostgreSQL with persistent data
gocker run -d --name db -p 5432:5432 -e POSTGRES_PASSWORD=secret postgres:16
`)

	// --- Workspace context ---
	fmt.Println("## Current workspace context")
	fmt.Println()

	cwd, _ := os.Getwd()
	fmt.Printf("Working directory: %s\n", cwd)

	// Detect compose files
	composeFiles := []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
	for _, f := range composeFiles {
		if _, err := os.Stat(filepath.Join(cwd, f)); err == nil {
			fmt.Printf("Compose file found: %s\n", f)
		}
	}

	// Detect Dockerfile
	if _, err := os.Stat(filepath.Join(cwd, "Dockerfile")); err == nil {
		fmt.Println("Dockerfile found: yes")
	}

	// Detect .env
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err == nil {
		fmt.Println("Env file found: .env")
	}

	// Show running containers
	containers, err := eng.ContainerList(ctx, false)
	if err == nil && len(containers) > 0 {
		fmt.Printf("\nRunning containers: %d\n", len(containers))
		for _, c := range containers {
			name := c.Name
			if name == "" {
				name = c.ID
			}
			parts := []string{name}
			if c.Image != "" {
				parts = append(parts, c.Image)
			}
			if c.Status != "" {
				parts = append(parts, c.Status)
			}
			fmt.Printf("  - %s\n", strings.Join(parts, " | "))
		}
	}

	fmt.Println()
}
