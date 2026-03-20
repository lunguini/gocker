package cmd

import (
	"context"
	"os"

	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/format"
	"github.com/urfave/cli/v3"
)

func newImagesCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:  "images",
		Usage: "List images",
		Action: func(ctx context.Context, cmd *cli.Command) error {
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
		},
	}
}
