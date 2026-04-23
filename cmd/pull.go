package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newPullCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:      "pull",
		Usage:     "Pull an image from a registry",
		ArgsUsage: "IMAGE",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "platform",
				Usage: "Pull only this platform (e.g. linux/arm64)",
			},
			&cli.IntFlag{
				Name:    "max-concurrent-downloads",
				Aliases: []string{"j"},
				Usage:   "Max concurrent layer downloads (Apple backend only; default: 3)",
			},
			&cli.StringFlag{
				Name:  "progress",
				Usage: "Progress output: ansi | none (default: auto-detect from TTY)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			image := cmd.Args().First()
			if image == "" {
				return cli.Exit("requires image name", 1)
			}
			return eng.ImagePull(ctx, image, engine.ImagePullOpts{
				Platform:      cmd.String("platform"),
				MaxConcurrent: int(cmd.Int("max-concurrent-downloads")),
				Progress:      cmd.String("progress"),
			})
		},
	}
}
