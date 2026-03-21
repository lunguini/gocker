package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func NewApp(version string) *cli.Command {
	eng := engine.New("")

	return &cli.Command{
		Name:    "gocker",
		Usage:   "Docker-compatible CLI for Apple Container on macOS",
		Version: version,
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
			newBuildCmd(eng),
			newDaemonCmd(eng),
			newExecCmd(eng),
			newImagesCmd(eng),
			newInspectCmd(eng),
			newLogsCmd(eng),
			newNetworkCmd(eng),
			newPsCmd(eng),
			newPullCmd(eng),
			newPushCmd(eng),
			newRmCmd(eng),
			newRmiCmd(eng),
			newRunCmd(eng),
			newSandboxCmd(eng),
			newSetupCmd(eng),
			newStartCmd(eng),
			newStopCmd(eng),
			newSystemCmd(eng),
			newVolumeCmd(eng),
		},
	}
}
