package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newRunCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "Run a container",
		ArgsUsage: "IMAGE [COMMAND] [ARG...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "interactive", Aliases: []string{"i"}, Usage: "Keep STDIN open"},
			&cli.BoolFlag{Name: "tty", Aliases: []string{"t"}, Usage: "Allocate a pseudo-TTY"},
			&cli.BoolFlag{Name: "detach", Aliases: []string{"d"}, Usage: "Run in background"},
			&cli.StringFlag{Name: "name", Usage: "Container name"},
			&cli.StringSliceFlag{Name: "volume", Aliases: []string{"v"}, Usage: "Bind mount a volume"},
			&cli.StringSliceFlag{Name: "publish", Aliases: []string{"p"}, Usage: "Publish a port"},
			&cli.StringSliceFlag{Name: "env", Aliases: []string{"e"}, Usage: "Set environment variables"},
			&cli.StringFlag{Name: "env-file", Usage: "Read env vars from file"},
			&cli.StringFlag{Name: "workdir", Aliases: []string{"w"}, Usage: "Working directory inside the container"},
			&cli.BoolFlag{Name: "rm", Usage: "Remove container when it exits"},
			&cli.StringFlag{Name: "network", Usage: "Connect to a network"},
			&cli.StringFlag{Name: "platform", Usage: "Set platform (e.g., linux/amd64)"},
			&cli.StringFlag{Name: "restart", Usage: "Restart policy (no, always, on-failure, unless-stopped)"},
			&cli.StringFlag{Name: "hostname", Aliases: []string{"h"}, Usage: "Container hostname"},
			&cli.StringFlag{Name: "cpus", Aliases: []string{"c"}, Usage: "Number of CPUs"},
			&cli.StringFlag{Name: "memory", Aliases: []string{"m"}, Usage: "Memory limit"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args := buildRunArgs(cmd)
			interactive := cmd.Bool("interactive") || cmd.Bool("tty")
			return eng.ContainerRun(ctx, args, interactive)
		},
	}
}

func buildRunArgs(cmd *cli.Command) []string {
	var args []string

	if cmd.Bool("interactive") {
		args = append(args, "-i")
	}
	if cmd.Bool("tty") {
		args = append(args, "-t")
	}
	if cmd.Bool("detach") {
		args = append(args, "-d")
	}
	if name := cmd.String("name"); name != "" {
		args = append(args, "--name", name)
	}
	for _, v := range cmd.StringSlice("volume") {
		args = append(args, "-v", v)
	}
	for _, p := range cmd.StringSlice("publish") {
		args = append(args, "-p", p)
	}
	for _, e := range cmd.StringSlice("env") {
		args = append(args, "-e", e)
	}
	if envFile := cmd.String("env-file"); envFile != "" {
		for _, e := range readEnvFile(envFile) {
			args = append(args, "-e", e)
		}
	}
	if workdir := cmd.String("workdir"); workdir != "" {
		args = append(args, "-w", workdir)
	}
	if cmd.Bool("rm") {
		args = append(args, "--rm")
	}
	if network := cmd.String("network"); network != "" {
		args = append(args, "--network", network)
	}
	if platform := cmd.String("platform"); platform != "" {
		args = append(args, "--platform", platform)
	}
	if restart := cmd.String("restart"); restart != "" {
		fmt.Fprintf(os.Stderr, "Warning: --restart=%s is not supported by Apple Container CLI (ignored). Container will not auto-restart.\n", restart)
	}
	if hostname := cmd.String("hostname"); hostname != "" {
		args = append(args, "--hostname", hostname)
	}
	if cpus := cmd.String("cpus"); cpus != "" {
		args = append(args, "--cpus", cpus)
	}
	if memory := cmd.String("memory"); memory != "" {
		args = append(args, "--memory", memory)
	}

	args = append(args, cmd.Args().Slice()...)
	return args
}

func readEnvFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var envs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		envs = append(envs, line)
	}
	return envs
}
