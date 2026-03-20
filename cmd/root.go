package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func NewApp() *cli.Command {
	eng := engine.New("")

	return &cli.Command{
		Name:    "gocker",
		Usage:   "Docker-compatible CLI for Apple Container on macOS",
		Version: "0.1.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Usage: "Output format (table, json)",
				Value: "table",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug output",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// Skip validation for setup (it installs the binary) and
			// when no subcommand is given (help/version output)
			first := cmd.Args().First()
			if first == "setup" || first == "" {
				return ctx, nil
			}
			return ctx, eng.Validate()
		},
		Commands: []*cli.Command{
			newRunCmd(eng),
			newPsCmd(eng),
			newStopCmd(eng),
			newRmCmd(eng),
			newStartCmd(eng),
			newExecCmd(eng),
			newLogsCmd(eng),
			newInspectCmd(eng),
			newPullCmd(eng),
			newImagesCmd(eng),
			newRmiCmd(eng),
			newBuildCmd(eng),
			newPushCmd(eng),
			newNetworkCmd(eng),
			newVolumeCmd(eng),
			newSystemCmd(eng),
			newDaemonCmd(eng),
			newSandboxCmd(eng),
			newSetupCmd(eng),
		},
	}
}
