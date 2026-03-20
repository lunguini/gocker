package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newExecCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Usage:     "Execute a command in a running container",
		ArgsUsage: "CONTAINER COMMAND [ARG...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "interactive", Aliases: []string{"i"}, Usage: "Keep STDIN open"},
			&cli.BoolFlag{Name: "tty", Aliases: []string{"t"}, Usage: "Allocate a pseudo-TTY"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args := cmd.Args().Slice()
			if len(args) < 2 {
				return cli.Exit("requires at least 2 arguments: CONTAINER COMMAND", 1)
			}
			container := args[0]
			execArgs := args[1:]
			interactive := cmd.Bool("interactive") || cmd.Bool("tty")

			var fullArgs []string
			if cmd.Bool("interactive") {
				fullArgs = append(fullArgs, "-i")
			}
			if cmd.Bool("tty") {
				fullArgs = append(fullArgs, "-t")
			}
			fullArgs = append(fullArgs, execArgs...)

			return eng.ContainerExec(ctx, container, fullArgs, interactive)
		},
	}
}
