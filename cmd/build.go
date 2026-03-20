package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newBuildCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:      "build",
		Usage:     "Build an image from a Dockerfile",
		ArgsUsage: "PATH",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "tag", Aliases: []string{"t"}, Usage: "Name and optionally a tag (name:tag)"},
			&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Name of the Dockerfile"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			var args []string
			if tag := cmd.String("tag"); tag != "" {
				args = append(args, "-t", tag)
			}
			if file := cmd.String("file"); file != "" {
				args = append(args, "-f", file)
			}
			args = append(args, cmd.Args().Slice()...)
			return eng.ImageBuild(ctx, args)
		},
	}
}
