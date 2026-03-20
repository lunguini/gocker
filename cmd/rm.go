package cmd

import (
	"context"
	"fmt"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newRmCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "Remove a container",
		ArgsUsage: "CONTAINER [CONTAINER...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Force removal of running container"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			for _, name := range cmd.Args().Slice() {
				if err := eng.ContainerRemove(ctx, name, cmd.Bool("force")); err != nil {
					return err
				}
				fmt.Println(name)
			}
			return nil
		},
	}
}
