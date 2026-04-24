package cmd

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func infoAction(eng, appleRT engine.Runtime, version string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() > 0 {
			return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
		}
		cfg := config.Load()
		isolation := cfg.IsolationFor("", cmd.Root().String("isolation"))
		if isolation == "" {
			isolation = "full"
		}

		fmt.Printf("Gocker version: %s\n", version)
		fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Isolation: %s\n", isolation)
		fmt.Printf("Container binary: %s\n", appleRT.BinaryPath())

		// Query the Apple CLI directly — eng may be SharedVMRuntime, which
		// would proxy `version` into the VM and return gocker's own version
		// instead of the Apple container CLI version.
		if stdout, _, err := appleRT.Exec(ctx, "--version"); err == nil {
			line := strings.TrimSpace(string(stdout))
			if line != "" {
				fmt.Printf("Container CLI version: %s\n", line)
			}
		}

		containers, _ := eng.ContainerList(ctx, true)
		running := 0
		for _, c := range containers {
			if strings.HasPrefix(strings.ToLower(c.Status), "up") || strings.EqualFold(c.Status, "running") {
				running++
			}
		}
		fmt.Printf("Containers: %d (running: %d)\n", len(containers), running)

		images, _ := eng.ImageList(ctx)
		fmt.Printf("Images: %d\n", len(images))

		return nil
	}
}

func newInfoCmd(eng, appleRT engine.Runtime, version string) *cli.Command {
	return &cli.Command{
		Name:   "info",
		Usage:  "Display system-wide information",
		Action: infoAction(eng, appleRT, version),
	}
}

func newSystemCmd(eng, appleRT engine.Runtime, version string) *cli.Command {
	return &cli.Command{
		Name:  "system",
		Usage: "Manage gocker system",
		Commands: []*cli.Command{
			{
				Name:   "info",
				Usage:  "Display system-wide information",
				Action: infoAction(eng, appleRT, version),
			},
			{
				Name:  "prune",
				Usage: "Remove unused containers, networks, and dangling images",
				Description: `Remove unused data. By default:
  - stopped containers
  - unused networks (not connected to any container)
  - dangling images (no repo:tag)

Use --volumes to also remove all unused volumes.
Use -a/--all to remove all unused images (not just dangling).

Note: 'unused' means the backend will let us remove it. Resources attached
to a running container are silently skipped — stop those containers first
if you need them gone. -f/--force skips the confirmation prompt; it does
NOT force-delete in-use resources (matching Docker's behavior).`,
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
						Usage:   "Do not prompt for confirmation",
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
