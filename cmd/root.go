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
	generalRTDirect := appleRT // for run, ps, exec, stop, rm, etc.
	sandboxRTDirect := appleRT // for sandbox commands (always full in hybrid)

	// In hybrid/shared mode, create a SharedVM runtime for general commands
	if isolation == "hybrid" || isolation == "shared" {
		mgr := sharedvm.NewManager(appleRT, cfg.SharedVM)
		sharedRT := sharedvm.NewSharedVMRuntime(mgr, appleRT)

		generalRTDirect = sharedRT

		// Sandbox: only shared in explicit "shared" mode
		if cfg.IsolationFor("sandbox", "") == "shared" {
			sandboxRTDirect = sharedRT
		}
	}

	// generalRT/sandboxRT are runtimeSwitch instances, not the concrete
	// runtimes above, so that the Before hook (which runs after flag
	// parsing, unlike this constructor) can swap the underlying runtime
	// when --isolation overrides the config-resolved mode (H5). Command
	// constructors receive the switch and never see the swap happen.
	generalRT := newRuntimeSwitch(generalRTDirect)
	sandboxRT := newRuntimeSwitch(sandboxRTDirect)

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
				Usage: "Isolation mode: full, hybrid, shared (overrides ~/.gocker/config.yaml for this invocation)",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			first := cmd.Args().First()

			// --isolation re-resolves the runtimes behind generalRT/sandboxRT
			// (both runtimeSwitch instances) now that flags are parsed. This
			// runs even for "setup"/"ai"/"" so `gocker --isolation shared ai`
			// reflects the override in its printed context, though those
			// commands skip the validation/auto-start below.
			if cmd.IsSet("isolation") {
				override := cmd.String("isolation")
				switch override {
				case "full", "hybrid", "shared":
				default:
					return ctx, cli.Exit(fmt.Sprintf("invalid --isolation value %q (must be full, hybrid, or shared)", override), 1)
				}

				if override != isolation {
					var newGeneral, newSandbox engine.Runtime = appleRT, appleRT
					if override == "hybrid" || override == "shared" {
						mgr := sharedvm.NewManager(appleRT, cfg.SharedVM)
						sharedRT := sharedvm.NewSharedVMRuntime(mgr, appleRT)
						newGeneral = sharedRT
						if cfg.IsolationFor("sandbox", override) == "shared" {
							newSandbox = sharedRT
						}
					}
					generalRT.Store(newGeneral)
					sandboxRT.Store(newSandbox)
				}
			}

			if first == "setup" || first == "ai" || first == "" {
				return ctx, nil
			}

			// Warn if sandbox isolation is downgraded, whether via
			// --isolation or ~/.gocker/config.yaml.
			if first == "sandbox" && cfg.IsolationFor("sandbox", cmd.String("isolation")) == "shared" {
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
