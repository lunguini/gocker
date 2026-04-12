package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lunguini/gocker/compose"
	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/sharedvm"
	"github.com/urfave/cli/v3"
)

func newComposeCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:            "compose",
		Usage:           "Manage multi-container applications with Compose",
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

// addNoTTYIfNeeded inserts -T after "exec" when stdin is not a terminal,
// so nerdctl compose exec doesn't try to allocate a TTY.
func addNoTTYIfNeeded(args []string) []string {
	if isTerminal() {
		return args
	}
	var result []string
	for i, a := range args {
		result = append(result, a)
		if a == "exec" && (i == 0 || !strings.HasPrefix(args[i-1], "-")) {
			result = append(result, "-T")
		}
	}
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
		if strings.HasPrefix(a, "--project-name=") {
			return strings.TrimPrefix(a, "--project-name=")
		}
	}
	return ""
}

func isInteractiveCompose(args []string) bool {
	// Only interactive if stdin is a terminal. Harbor runs exec with
	// stdin=DEVNULL, so we shouldn't force -it on the outer container exec.
	for _, a := range args {
		if a == "exec" || a == "run" {
			return isTerminal()
		}
	}
	return false
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func isComposeDown(args []string) bool {
	for _, a := range args {
		if a == "down" {
			return true
		}
	}
	return false
}
