package cmd

import (
	"context"
	"fmt"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newInspectCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:      "inspect",
		Usage:     "Return low-level information on a container",
		ArgsUsage: "CONTAINER",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name := cmd.Args().First()
			if name == "" {
				return cli.Exit("requires container name or ID", 1)
			}
			data, err := eng.ContainerInspect(ctx, name)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	}
}
