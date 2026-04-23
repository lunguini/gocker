package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/format"
	"github.com/urfave/cli/v3"
)

func newImagesCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "images",
		Usage: "List images",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 0 {
				return cli.Exit(fmt.Sprintf(
					"unexpected arguments: %v\n\nTo remove an image: gocker rmi <image> (or gocker image rm <image>)",
					cmd.Args().Slice(),
				), 2)
			}
			return listImages(ctx, cmd, eng)
		},
	}
}

// newImageCmd is the Docker-style nested command group: `gocker image <verb>`.
// Provides ls / rm / pull / inspect / push as siblings to the flat top-level
// commands (images / rmi / pull / inspect / push) so both discovery paths work.
func newImageCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "image",
		Usage: "Manage images",
		Commands: []*cli.Command{
			{
				Name:  "ls",
				Usage: "List images",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() > 0 {
						return cli.Exit(fmt.Sprintf(
							"unexpected arguments: %v\n\nTo remove an image: gocker image rm <image>",
							cmd.Args().Slice(),
						), 2)
					}
					return listImages(ctx, cmd, eng)
				},
			},
			{
				Name:      "rm",
				Usage:     "Remove one or more images",
				ArgsUsage: "IMAGE [IMAGE...]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						return cli.Exit("requires at least one image name", 2)
					}
					return removeImages(ctx, eng, cmd.Args().Slice())
				},
			},
		},
	}
}

func listImages(ctx context.Context, cmd *cli.Command, eng engine.Runtime) error {
	images, err := eng.ImageList(ctx)
	if err != nil {
		return err
	}

	if cmd.Root().String("format") == "json" {
		return format.JSON(os.Stdout, images)
	}

	headers := []string{"REPOSITORY", "TAG", "IMAGE ID", "CREATED", "SIZE"}
	var rows [][]string
	for _, img := range images {
		rows = append(rows, []string{
			img.Name,
			img.Tag,
			format.TruncateID(img.ID),
			format.HumanDuration(img.Created),
			img.Size,
		})
	}
	format.Table(os.Stdout, headers, rows)
	return nil
}
