package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newLogsCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "logs",
		Usage:     "Fetch the logs of a container",
		ArgsUsage: "CONTAINER",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "Follow log output"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name := cmd.Args().First()
			if name == "" {
				return cli.Exit("requires container name or ID", 1)
			}
			return eng.ContainerLogs(ctx, name, cmd.Bool("follow"))
		},
	}
}
