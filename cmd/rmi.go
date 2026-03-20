package cmd

import (
	"context"
	"fmt"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newRmiCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "rmi",
		Usage:     "Remove an image",
		ArgsUsage: "IMAGE [IMAGE...]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			for _, image := range cmd.Args().Slice() {
				if err := eng.ImageRemove(ctx, image); err != nil {
					return err
				}
				fmt.Println("Deleted:", image)
			}
			return nil
		},
	}
}
