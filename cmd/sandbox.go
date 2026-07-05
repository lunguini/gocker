package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
					&cli.StringFlag{Name: "network-policy", Value: "allow", Usage: "Network policy: allow (default) or deny. deny is not enforced yet and will error — tracked for a future release"},
					&cli.StringSliceFlag{Name: "allow-host", Usage: "Allowed hosts (reserved for when --network-policy deny is enforced; has no effect today)"},
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
						return cli.Exit("requires agent name (e.g., claude, custom)", 1)
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

					networkPolicy := cmd.String("network-policy")
					if networkPolicy != "allow" && networkPolicy != "deny" {
						return cli.Exit(fmt.Sprintf("invalid --network-policy %q: must be \"allow\" or \"deny\"", networkPolicy), 1)
					}
					if networkPolicy == "deny" {
						return cli.Exit("--network-policy deny is not implemented yet: gocker cannot currently restrict sandbox network access, so it refuses to silently grant unrestricted access while claiming otherwise. Enforcement is tracked for a future release; omit the flag (or pass --network-policy allow) to run with unrestricted network access today.", 1)
					}

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
					if cmd.Args().Len() > 0 {
						return cli.Exit("unexpected arguments: "+strings.Join(cmd.Args().Slice(), " "), 2)
					}
					// Verify live status via a cheap inspect per sandbox rather
					// than trusting last-known state, which goes stale if a
					// sandbox exits/crashes outside gocker's control.
					sandboxes, err := mgr.ListLive(ctx)
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
