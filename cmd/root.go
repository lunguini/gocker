package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/sharedvm"
	"github.com/urfave/cli/v3"
)

const (
	defaultBaseLatestImage = "docker.io/adyjay/gocker:base-latest"
	defaultBaseDevImage    = "docker.io/adyjay/gocker:base-dev"
)

func NewApp(version string) *cli.Command {
	cfg := config.Load()

	// Dev builds (go install @main or local `make build` between tags) pull
	// :base-dev instead of :base-latest by default. :base-dev tracks main
	// and is pushed on every main commit; :base-latest only moves on a
	// tagged release. Users can still override explicitly in config.yaml.
	if config.IsDevVersion(version) && cfg.SharedVM.Image == defaultBaseLatestImage {
		cfg.SharedVM.Image = defaultBaseDevImage
	}

	// Auto-detect runtime based on platform and config
	appleRT, detectErr := engine.DetectRuntime(cfg.RuntimeBinary())
	if detectErr != nil {
		appleRT = engine.New("")
	}

	// Resolve isolation mode and build runtimes
	isolation := cfg.Isolation
	if isolation == "" {
		isolation = "full"
	}

	// Default: everything uses the direct runtime
	generalRT := appleRT // for run, ps, exec, stop, rm, etc.
	sandboxRT := appleRT // for sandbox commands (always full in hybrid)

	// In hybrid/shared mode, create a SharedVM runtime for general commands
	if isolation == "hybrid" || isolation == "shared" {
		mgr := sharedvm.NewManager(appleRT, cfg.SharedVM)
		sharedRT := sharedvm.NewSharedVMRuntime(mgr, appleRT)

		generalRT = sharedRT

		// Sandbox: only shared in explicit "shared" mode
		if cfg.IsolationFor("sandbox", "") == "shared" {
			sandboxRT = sharedRT
		}
	}

	return &cli.Command{
		Name:                   "gocker",
		Usage:                  "Docker-compatible CLI for Apple Container on macOS",
		Version:                version,
		EnableShellCompletion:  true,
		UseShortOptionHandling: true,
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
			&cli.StringFlag{
				Name:  "isolation",
				Usage: "Isolation mode: full, hybrid, shared (compose only; other commands read ~/.gocker/config.yaml)",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			first := cmd.Args().First()
			if first == "setup" || first == "ai" || first == "" {
				return ctx, nil
			}

			// --isolation only takes effect for `compose` today: runtimes for
			// every other command are built once in NewApp, before flags are
			// parsed, so re-resolving them here would require plumbing a
			// mutable runtime indirection through every command constructor.
			// Rather than silently ignore the flag (H5), refuse explicitly so
			// users aren't misled into thinking it took effect; ~/.gocker/config.yaml
			// is the supported way to change isolation mode for these commands.
			if cmd.IsSet("isolation") && first != "compose" {
				return ctx, cli.Exit(fmt.Sprintf("--isolation is not yet supported for %q commands (only `gocker compose`); set \"isolation\" in ~/.gocker/config.yaml instead", first), 1)
			}

			// Warn if sandbox isolation is downgraded. --isolation can't
			// reach here for "sandbox" (rejected above), so only the
			// config-file mode is relevant.
			if first == "sandbox" && cfg.IsolationFor("sandbox", "") == "shared" {
				fmt.Fprintln(os.Stderr, "⚠ Running sandbox in shared isolation mode. Agent has kernel-level access to other containers. Use \"isolation: full\" in ~/.gocker/config.yaml for hardware isolation.")
			}

			if err := appleRT.Validate(); err != nil {
				return ctx, err
			}

			// Auto-start Apple Container system service if it's not running.
			if eng, ok := appleRT.(*engine.Engine); ok {
				if err := eng.EnsureSystemRunning(ctx); err != nil {
					return ctx, err
				}
			}

			return ctx, nil
		},
		Commands: []*cli.Command{
			newAICmd(generalRT),
			newBuildCmd(generalRT),
			newComposeCmd(appleRT),  // compose proxies to nerdctl inside a VM
			newDaemonCmd(generalRT), // uses SharedVMRuntime in shared/hybrid mode
			newDoctorCmd(),
			newExecCmd(generalRT),
			newImageCmd(generalRT),
			newImagesCmd(generalRT),
			newInfoCmd(generalRT, appleRT, version),
			newInspectCmd(generalRT),
			newLogsCmd(generalRT),
			newNetworkCmd(generalRT),
			newPsCmd(generalRT),
			newPullCmd(generalRT),
			newPushCmd(generalRT),
			newRmCmd(generalRT),
			newRmiCmd(generalRT),
			newRunCmd(generalRT),
			newSandboxCmd(sandboxRT),
			newSetupCmd(appleRT), // setup always runs directly
			newStartCmd(generalRT),
			newStopCmd(generalRT),
			newSystemCmd(generalRT, appleRT, version),
			newVolumeCmd(generalRT),
		},
	}
}
