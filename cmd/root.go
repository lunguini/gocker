package cmd

import (
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
		},
	}
}
