package cmd

import (
	"context"
	"fmt"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newStartCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "start",
		Usage:     "Start a stopped container",
		ArgsUsage: "CONTAINER [CONTAINER...]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			for _, name := range cmd.Args().Slice() {
				if err := eng.ContainerStart(ctx, name); err != nil {
					return err
				}
				fmt.Println(name)
			}
			return nil
		},
	}
}
