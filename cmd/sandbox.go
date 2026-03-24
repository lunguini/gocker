package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/format"
	"github.com/lunguini/gocker/sandbox"
	"github.com/urfave/cli/v3"
)

func newSandboxCmd(eng engine.Runtime) *cli.Command {
	mgr := sandbox.NewManager(eng)

	return &cli.Command{
		Name:  "sandbox",
		Usage: "AI agent sandboxing with hardware-level isolation",
		Commands: []*cli.Command{
			{
				Name:      "run",
				Usage:     "Create and run an agent sandbox",
				ArgsUsage: "AGENT [WORKSPACE]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "Custom sandbox name"},
					&cli.StringFlag{Name: "network-policy", Value: "allow", Usage: "Network policy: allow or deny"},
					&cli.StringSliceFlag{Name: "allow-host", Usage: "Allowed hosts when policy is deny"},
					&cli.StringSliceFlag{Name: "env", Aliases: []string{"e"}, Usage: "Additional environment variables"},
					&cli.StringFlag{Name: "image", Usage: "Override template image"},
					&cli.StringFlag{Name: "entrypoint", Usage: "Override entry command"},
					&cli.BoolFlag{Name: "detach", Aliases: []string{"d"}, Usage: "Run in background"},
					&cli.BoolFlag{Name: "sync-config", Value: true, Usage: "Sync host agent config into sandbox"},
					&cli.BoolFlag{Name: "sync-state", Value: true, Usage: "Sync ~/.claude.json into sandbox"},
					&cli.BoolFlag{Name: "no-managed-settings", Usage: "Skip mounting enterprise managed settings"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					agent := cmd.Args().First()
					if agent == "" {
						return cli.Exit("requires agent name (e.g., claude, codex, custom)", 1)
					}

					workspace := cmd.Args().Get(1)
					if workspace == "" {
						var err error
						workspace, err = os.Getwd()
						if err != nil {
							return err
						}
					}
					workspace, _ = filepath.Abs(workspace)

					name := cmd.String("name")
					if name == "" {
						name = agent + "-" + filepath.Base(workspace)
					}

					cfg := config.Load()
					opts := sandbox.RunOptions{
						Name:            name,
						Agent:           agent,
						Workspace:       workspace,
						NetworkPolicy:   cmd.String("network-policy"),
						AllowedHosts:    cmd.StringSlice("allow-host"),
						ExtraEnv:        cmd.StringSlice("env"),
						ImageOverride:   cmd.String("image"),
						EntryOverride:   cmd.String("entrypoint"),
						Detach:          cmd.Bool("detach"),
						SyncConfig:      cmd.Bool("sync-config"),
						SyncState:       cmd.Bool("sync-state"),
						SyncSession:     agent == "claude" && cfg.SyncClaudeSession(),
						ManagedSettings: !cmd.Bool("no-managed-settings"),
					}

					return mgr.Run(ctx, opts)
				},
			},
			{
				Name:    "ls",
				Aliases: []string{"list"},
				Usage:   "List sandboxes",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					sandboxes, err := mgr.List()
					if err != nil {
						return err
					}
					if cmd.Root().String("format") == "json" {
						return format.JSON(os.Stdout, sandboxes)
					}
					headers := []string{"NAME", "AGENT", "STATUS", "WORKSPACE", "CONTAINER"}
					var rows [][]string
					for _, s := range sandboxes {
						rows = append(rows, []string{
							s.Name,
							s.Agent,
							s.Status,
							s.Workspace,
							format.TruncateID(s.ContainerID),
						})
					}
					format.Table(os.Stdout, headers, rows)
					return nil
				},
			},
			{
				Name:      "stop",
				Usage:     "Stop a sandbox",
				ArgsUsage: "NAME",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires sandbox name", 1)
					}
					if err := mgr.Stop(ctx, name); err != nil {
						return err
					}
					fmt.Println(name)
					return nil
				},
			},
			{
				Name:      "rm",
				Aliases:   []string{"remove"},
				Usage:     "Remove a sandbox",
				ArgsUsage: "NAME",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires sandbox name", 1)
					}
					if err := mgr.Remove(ctx, name); err != nil {
						return err
					}
					fmt.Println(name)
					return nil
				},
			},
			{
				Name:      "attach",
				Usage:     "Attach to a running sandbox",
				ArgsUsage: "NAME",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires sandbox name", 1)
					}
					return mgr.Attach(ctx, name)
				},
			},
			{
				Name:      "logs",
				Usage:     "Fetch sandbox logs",
				ArgsUsage: "NAME",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "follow", Aliases: []string{"f"}, Usage: "Follow log output"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return cli.Exit("requires sandbox name", 1)
					}
					return mgr.Logs(ctx, name, cmd.Bool("follow"))
				},
			},
		},
	}
}
