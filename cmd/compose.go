package cmd

import (
	"context"
	"os"

	"github.com/lunguini/gocker/compose"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/format"
	"github.com/urfave/cli/v3"
)

func newComposeCmd(eng *engine.Engine) *cli.Command {
	orch := compose.NewOrchestrator(eng)

	return &cli.Command{
		Name:  "compose",
		Usage: "Manage multi-container applications with Compose",
		Commands: []*cli.Command{
			{
				Name:      "up",
				Usage:     "Create and start services",
				ArgsUsage: "",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Compose file path"},
					&cli.BoolFlag{Name: "detach", Aliases: []string{"d"}, Usage: "Run in background (default for compose)", Value: true},
					&cli.StringFlag{Name: "project-name", Aliases: []string{"p"}, Usage: "Project name override"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return orch.Up(ctx, compose.UpOptions{
						File:    cmd.String("file"),
						Detach:  cmd.Bool("detach"),
						Project: cmd.String("project-name"),
					})
				},
			},
			{
				Name:      "down",
				Usage:     "Stop and remove services, networks",
				ArgsUsage: "",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Compose file path"},
					&cli.StringFlag{Name: "project-name", Aliases: []string{"p"}, Usage: "Project name override"},
					&cli.BoolFlag{Name: "volumes", Aliases: []string{"v"}, Usage: "Also remove named volumes"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return orch.Down(ctx, compose.DownOptions{
						File:    cmd.String("file"),
						Project: cmd.String("project-name"),
						Volumes: cmd.Bool("volumes"),
					})
				},
			},
			{
				Name:      "ps",
				Usage:     "List service containers",
				ArgsUsage: "",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Compose file path"},
					&cli.StringFlag{Name: "project-name", Aliases: []string{"p"}, Usage: "Project name override"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					statuses, err := orch.Ps(ctx, compose.PsOptions{
						File:    cmd.String("file"),
						Project: cmd.String("project-name"),
					})
					if err != nil {
						return err
					}
					if cmd.Root().String("format") == "json" {
						return format.JSON(os.Stdout, statuses)
					}
					headers := []string{"SERVICE", "CONTAINER", "IMAGE", "STATUS"}
					var rows [][]string
					for _, s := range statuses {
						rows = append(rows, []string{
							s.Service,
							s.Container,
							s.Image,
							s.Status,
						})
					}
					format.Table(os.Stdout, headers, rows)
					return nil
				},
			},
			{
				Name:      "logs",
				Usage:     "View output from services",
				ArgsUsage: "[SERVICE]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Compose file path"},
					&cli.StringFlag{Name: "project-name", Aliases: []string{"p"}, Usage: "Project name override"},
					&cli.BoolFlag{Name: "follow", Aliases: []string{"F"}, Usage: "Follow log output"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return orch.Logs(ctx, compose.LogsOptions{
						File:    cmd.String("file"),
						Project: cmd.String("project-name"),
						Service: cmd.Args().First(),
						Follow:  cmd.Bool("follow"),
					})
				},
			},
			{
				Name:      "restart",
				Usage:     "Restart services",
				ArgsUsage: "[SERVICE]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Usage: "Compose file path"},
					&cli.StringFlag{Name: "project-name", Aliases: []string{"p"}, Usage: "Project name override"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return orch.Restart(ctx, compose.RestartOptions{
						File:    cmd.String("file"),
						Project: cmd.String("project-name"),
						Service: cmd.Args().First(),
					})
				},
			},
		},
	}
}
