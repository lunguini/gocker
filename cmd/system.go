package cmd

import (
	"context"
	"fmt"
	"runtime"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newSystemCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:  "system",
		Usage: "Manage gocker system",
		Commands: []*cli.Command{
			{
				Name:  "info",
				Usage: "Display system-wide information",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Println("Gocker version: 0.1.0")
					fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
					fmt.Printf("Container binary: %s\n", eng.Binary)

					// Get container CLI version
					stdout, _, err := eng.Exec(ctx, "version")
					if err == nil {
						fmt.Printf("Container CLI version: %s", string(stdout))
					}

					containers, _ := eng.ContainerList(ctx, true)
					fmt.Printf("Containers: %d\n", len(containers))

					images, _ := eng.ImageList(ctx)
					fmt.Printf("Images: %d\n", len(images))

					return nil
				},
			},
			{
				Name:  "prune",
				Usage: "Remove stopped containers and unused images",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					containers, err := eng.ContainerList(ctx, true)
					if err != nil {
						return err
					}
					for _, c := range containers {
						if c.State == "stopped" || c.State == "exited" {
							if err := eng.ContainerRemove(ctx, c.ID, false); err != nil {
								fmt.Printf("Warning: failed to remove container %s: %v\n", c.Name, err)
							} else {
								fmt.Printf("Removed container: %s\n", c.Name)
							}
						}
					}
					return nil
				},
			},
		},
	}
}
