package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newRmiCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:      "rmi",
		Usage:     "Remove one or more images",
		ArgsUsage: "IMAGE [IMAGE...]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return cli.Exit("requires at least one image name", 2)
			}
			return removeImages(ctx, eng, cmd.Args().Slice())
		},
	}
}
