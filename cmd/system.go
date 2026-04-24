package cmd

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func infoAction(eng engine.Runtime) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() > 0 {
			return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
		}
		fmt.Println("Gocker version: 0.1.0")
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Container binary: %s\n", eng.BinaryPath())

		stdout, _, err := eng.Exec(ctx, "version")
		if err == nil {
			fmt.Printf("Container CLI version: %s", string(stdout))
		}

		containers, _ := eng.ContainerList(ctx, true)
		fmt.Printf("Containers: %d\n", len(containers))

		images, _ := eng.ImageList(ctx)
		fmt.Printf("Images: %d\n", len(images))

		return nil
	}
}

func newInfoCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:   "info",
		Usage:  "Display system-wide information",
		Action: infoAction(eng),
	}
}

func newSystemCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "system",
		Usage: "Manage gocker system",
		Commands: []*cli.Command{
			{
				Name:   "info",
				Usage:  "Display system-wide information",
				Action: infoAction(eng),
			},
			{
				Name:  "prune",
				Usage: "Remove unused containers, networks, and dangling images",
				Description: `Remove unused data. By default:
  - stopped containers
  - unused networks (not connected to any container)
  - dangling images (no repo:tag)

Use --volumes to also remove all unused volumes.
Use -a/--all to remove all unused images (not just dangling).`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "all",
						Aliases: []string{"a"},
						Usage:   "Remove all unused images, not just dangling ones",
					},
					&cli.BoolFlag{
						Name:  "volumes",
						Usage: "Also prune unused anonymous and named volumes",
					},
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage:   "Do not prompt for confirmation (currently always non-interactive)",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() > 0 {
						return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
					}
					c := pruneStoppedContainers(ctx, eng)
					n := pruneUnusedNetworks(ctx, eng)
					i := pruneImages(ctx, eng, cmd.Bool("all"))
					printPruneReport("containers", c)
					printPruneReport("networks", n)
					printPruneReport("images", i)
					if cmd.Bool("volumes") {
						v := pruneUnusedVolumes(ctx, eng)
						printPruneReport("volumes", v)
					} else {
						fmt.Println("Volumes were kept — pass --volumes to also prune them.")
					}
					return nil
				},
			},
		},
	}
}
