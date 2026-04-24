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

func newNetworkCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "network",
		Usage: "Manage networks",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Create a network",
				ArgsUsage: "NAME",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires network name", 1)
					}
					if err := eng.NetworkCreate(ctx, name); err != nil {
						return err
					}
					fmt.Println(name)
					return nil
				},
			},
			{
				Name:    "ls",
				Aliases: []string{"list"},
				Usage:   "List networks",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() > 0 {
						return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
					}
					networks, err := eng.NetworkList(ctx)
					if err != nil {
						return err
					}
					if cmd.Root().String("format") == "json" {
						return format.JSON(os.Stdout, networks)
					}
					headers := []string{"NETWORK ID", "NAME", "DRIVER", "SCOPE"}
					var rows [][]string
					for _, n := range networks {
						rows = append(rows, []string{
							format.TruncateID(n.ID),
							n.Name,
							n.Driver,
							n.Scope,
						})
					}
					format.Table(os.Stdout, headers, rows)
					return nil
				},
			},
			{
				Name:      "rm",
				Aliases:   []string{"remove"},
				Usage:     "Remove a network",
				ArgsUsage: "NETWORK [NETWORK...]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					for _, name := range cmd.Args().Slice() {
						if err := eng.NetworkRemove(ctx, name); err != nil {
							return err
						}
						fmt.Println(name)
					}
					return nil
				},
			},
			{
				Name:  "prune",
				Usage: "Remove all unused networks",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Do not prompt for confirmation"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() > 0 {
						return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
					}
					printPruneReport("networks", pruneUnusedNetworks(ctx, eng))
					return nil
				},
			},
			{
				Name:      "connect",
				Usage:     "Connect a container to a network",
				ArgsUsage: "NETWORK CONTAINER",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if len(args) < 2 {
						return cli.Exit("requires NETWORK and CONTAINER", 1)
					}
					return eng.NetworkConnect(ctx, args[0], args[1])
				},
			},
			{
				Name:      "disconnect",
				Usage:     "Disconnect a container from a network",
				ArgsUsage: "NETWORK CONTAINER",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if len(args) < 2 {
						return cli.Exit("requires NETWORK and CONTAINER", 1)
					}
					return eng.NetworkDisconnect(ctx, args[0], args[1])
				},
			},
			{
				Name:      "inspect",
				Usage:     "Display detailed network information",
				ArgsUsage: "NETWORK",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires network name", 1)
					}
					data, err := eng.NetworkInspect(ctx, name)
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
