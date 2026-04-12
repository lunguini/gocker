package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newExecCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:            "exec",
		Usage:           "Execute a command in a running container",
		ArgsUsage:       "CONTAINER COMMAND [ARG...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args := cmd.Args().Slice()

			// Parse -i, -t, -it flags manually from the front of args.
			interactive := false
			var flags []string
			for len(args) > 0 {
				switch args[0] {
				case "-i":
					interactive = true
					flags = append(flags, "-i")
					args = args[1:]
				case "-t":
					interactive = true
					flags = append(flags, "-t")
					args = args[1:]
				case "-it", "-ti":
					interactive = true
					flags = append(flags, "-i", "-t")
					args = args[1:]
				case "-d", "--detach":
					args = args[1:]
				case "-w", "--workdir", "-e", "--env", "-u", "--user":
					// Pass through flags with values
					if len(args) > 1 {
						flags = append(flags, args[0], args[1])
						args = args[2:]
					} else {
						args = args[1:]
					}
				default:
					goto done
				}
			}
		done:

			if len(args) < 2 {
				return cli.Exit("requires at least 2 arguments: CONTAINER COMMAND", 1)
			}
			container := args[0]
			execArgs := append(flags, args[1:]...)

			return eng.ContainerExec(ctx, container, execArgs, interactive)
		},
	}
}
