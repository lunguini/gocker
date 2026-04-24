package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/format"
	"github.com/urfave/cli/v3"
)

func newVolumeCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "volume",
		Usage: "Manage volumes",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Create a volume",
				ArgsUsage: "NAME",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires volume name", 1)
					}
					if err := eng.VolumeCreate(ctx, name); err != nil {
						return err
					}
					fmt.Println(name)
					return nil
				},
			},
			{
				Name:    "ls",
				Aliases: []string{"list"},
				Usage:   "List volumes",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() > 0 {
						return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
					}
					volumes, err := eng.VolumeList(ctx)
					if err != nil {
						return err
					}
					if cmd.Root().String("format") == "json" {
						return format.JSON(os.Stdout, volumes)
					}
					headers := []string{"DRIVER", "VOLUME NAME"}
					var rows [][]string
					for _, v := range volumes {
						rows = append(rows, []string{v.Driver, v.Name})
					}
					format.Table(os.Stdout, headers, rows)
					return nil
				},
			},
			{
				Name:      "rm",
				Aliases:   []string{"remove"},
				Usage:     "Remove a volume",
				ArgsUsage: "VOLUME [VOLUME...]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					for _, name := range cmd.Args().Slice() {
						if err := eng.VolumeRemove(ctx, name); err != nil {
							return err
						}
						fmt.Println(name)
					}
					return nil
				},
			},
			{
				Name:  "prune",
				Usage: "Remove all unused volumes",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Do not prompt for confirmation"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() > 0 {
						return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
					}
					printPruneReport("volumes", pruneUnusedVolumes(ctx, eng))
					return nil
				},
			},
			{
				Name:      "inspect",
				Usage:     "Display detailed volume information",
				ArgsUsage: "VOLUME",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires volume name", 1)
					}
					data, err := eng.VolumeInspect(ctx, name)
					if err != nil {
						return err
					}
					fmt.Print(string(data))
					return nil
				},
			},
		},
	}
}
