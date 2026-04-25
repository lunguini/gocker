package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newLogsCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:      "logs",
		Usage:     "Fetch the logs of a container",
		ArgsUsage: "CONTAINER",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "Follow log output"},
			&cli.StringFlag{Name: "tail", Aliases: []string{"n"}, Usage: "Number of lines to show from the end of the logs (default: all)"},
			&cli.StringFlag{Name: "since", Usage: "Show logs since timestamp (e.g. 2024-01-02T15:04:05) or relative (e.g. 42m)"},
			&cli.StringFlag{Name: "until", Usage: "Show logs before timestamp or relative duration"},
			&cli.BoolFlag{Name: "timestamps", Aliases: []string{"t"}, Usage: "Show timestamps"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name := cmd.Args().First()
			if name == "" {
				return cli.Exit("requires container name or ID", 1)
			}
			return eng.ContainerLogs(ctx, name, engine.LogsOptions{
				Follow:     cmd.Bool("follow"),
				Tail:       cmd.String("tail"),
				Since:      cmd.String("since"),
				Until:      cmd.String("until"),
				Timestamps: cmd.Bool("timestamps"),
			})
		},
	}
}
