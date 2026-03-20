package cmd

import (
	"context"
	"fmt"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newStopCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "stop",
		Usage:     "Stop a running container",
		ArgsUsage: "CONTAINER [CONTAINER...]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			for _, name := range cmd.Args().Slice() {
				if err := eng.ContainerStop(ctx, name); err != nil {
					return err
				}
				fmt.Println(name)
			}
			return nil
		},
	}
}
