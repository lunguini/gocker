package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newPullCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "pull",
		Usage:     "Pull an image from a registry",
		ArgsUsage: "IMAGE",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			image := cmd.Args().First()
			if image == "" {
				return cli.Exit("requires image name", 1)
			}
			return eng.ImagePull(ctx, image)
		},
	}
}
