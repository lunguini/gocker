package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lunguini/gocker/compose"
	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/internal/termx"
	"github.com/lunguini/gocker/sharedvm"
	"github.com/urfave/cli/v3"
)

func newComposeCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:            "compose",
		Usage:           "Manage multi-container applications with Compose",
		ArgsUsage:       "[SUBCOMMAND] [ARGS...]",
		SkipFlagParsing: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runComposeProxy(ctx, cmd, eng)
		},
	}
}

// runComposeProxy extracts raw args after "compose" from os.Args and proxies
// them to nerdctl compose inside a VM.
func runComposeProxy(ctx context.Context, cmd *cli.Command, eng engine.Runtime) error {
	cfg := config.Load()
	isolation := cfg.IsolationFor("compose", cmd.Root().String("isolation"))

	// Extract raw args after "compose" from os.Args.
	// This preserves all flags exactly as passed, including ones we don't know about.
	args := rawComposeArgs()

	// Strip flags nerdctl doesn't support and handle TTY.
	args = filterUnsupportedFlags(args)
	args = addNoTTYIfNeeded(args)

	var mgr *sharedvm.Manager
	switch isolation {
	case "shared", "hybrid":
		mgr = sharedvm.NewManager(eng, cfg.SharedVM)
	default: // full
		project := extractProjectName(nil, args)
		if project == "" {
			project = "default"
		}
		mgr = sharedvm.NewManagerWithName(eng, cfg.SharedVM, "gocker-compose-"+project)
	}

	p := compose.NewProxy(eng, mgr)
	interactive := isInteractiveCompose(args)

	if err := p.Exec(ctx, args, interactive); err != nil {
		return err
	}

	// In full mode, clean up the per-project VM after compose down.
	if isolation == "full" && isComposeDown(args) {
		fmt.Println("Removing compose VM...")
		_ = mgr.Remove(ctx)
	}

	return nil
}

// rawComposeArgs extracts everything after "compose" from os.Args.
func rawComposeArgs() []string {
	for i, arg := range os.Args {
		if arg == "compose" {
			return os.Args[i+1:]
		}
	}
	return nil
}

// addNoTTYIfNeeded inserts -T right after the compose subcommand when that
// subcommand is "exec" or "run" and stdin is not a terminal, so nerdctl
// compose doesn't try to allocate a TTY. It matches the subcommand position
// only — a service literally named "exec" passed as an argument won't trigger
// injection.
func addNoTTYIfNeeded(args []string) []string {
	if termx.StdinIsTTY() {
		return args
	}
	idx := composeSubcommandIndex(args)
	if idx < 0 {
		return args
	}
	switch args[idx] {
	case "exec", "run":
	default:
		return args
	}
	result := make([]string, 0, len(args)+1)
	result = append(result, args[:idx+1]...)
	result = append(result, "-T")
	result = append(result, args[idx+1:]...)
	return result
}

// filterUnsupportedFlags removes flags that nerdctl compose doesn't support.
func filterUnsupportedFlags(args []string) []string {
	var result []string
	skip := false
	for i, a := range args {
		if skip {
			skip = false
			continue
		}
		switch a {
		case "--wait":
			// nerdctl compose up doesn't support --wait
			continue
		case "--rmi":
			// nerdctl compose down doesn't support --rmi
			// Skip the flag and its value
			if i+1 < len(args) {
				skip = true
			}
			continue
		}
		// Handle --rmi=value form
		if strings.HasPrefix(a, "--rmi=") {
			continue
		}
		result = append(result, a)
	}
	return result
}

func extractProjectName(_ *cli.Command, args []string) string {
	for i, a := range args {
		if (a == "--project-name" || a == "-p") && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--project-name="); ok {
			return v
		}
	}
	return ""
}

func isInteractiveCompose(args []string) bool {
	// Only interactive if stdin is a terminal. Harbor runs exec with
	// stdin=DEVNULL, so we shouldn't force -it on the outer container exec.
	if idx := composeSubcommandIndex(args); idx >= 0 {
		switch args[idx] {
		case "exec", "run":
			return termx.StdinIsTTY()
		}
	}
	return false
}

func isComposeDown(args []string) bool {
	idx := composeSubcommandIndex(args)
	return idx >= 0 && args[idx] == "down"
}

// composeSubcommandIndex locates the compose subcommand token; the logic
// lives in the compose package (compose.SubcommandIndex) so the proxy can
// classify read-only subcommands with the same rules.
func composeSubcommandIndex(args []string) int {
	return compose.SubcommandIndex(args)
}
