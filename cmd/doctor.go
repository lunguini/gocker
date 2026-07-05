package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/internal/fsutil"
	"github.com/urfave/cli/v3"
)

type checkStatus int

const (
	statusOK checkStatus = iota
	statusWarn
	statusFail
)

type diagCheck struct {
	name   string
	detail string
	status checkStatus
}

// renderDiagnostics formats checks as an aligned pass/warn/fail list and
// reports overall health. Only failures mark the report unhealthy; warnings
// are advisory (e.g. the daemon not running is fine for CLI-only use).
func renderDiagnostics(checks []diagCheck) (out string, healthy bool) {
	healthy = true
	var b strings.Builder
	for _, c := range checks {
		var marker string
		switch c.status {
		case statusOK:
			marker = "✓"
		case statusWarn:
			marker = "!"
		case statusFail:
			marker = "✗"
			healthy = false
		}
		fmt.Fprintf(&b, "%s %-20s %s\n", marker, c.name, c.detail)
	}
	return b.String(), healthy
}

func newDoctorCmd() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "Diagnose gocker configuration and environment",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			checks := gatherDiagnostics()
			out, healthy := renderDiagnostics(checks)
			fmt.Print(out)
			if !healthy {
				fmt.Fprintln(os.Stderr, "\nSome checks failed — see the ✗ lines above.")
				return cli.Exit("", 1)
			}
			return nil
		},
	}
}

func gatherDiagnostics() []diagCheck {
	var checks []diagCheck

	// Platform.
	checks = append(checks, diagCheck{"Platform", runtime.GOOS + "/" + runtime.GOARCH, statusOK})

	// Config: report path and whether it parses.
	cfgPath := config.Path()
	cfg, cfgErr := config.LoadFrom(cfgPath)
	if cfgErr != nil {
		checks = append(checks, diagCheck{"Config", fmt.Sprintf("%s (%v — using defaults)", cfgPath, cfgErr), statusFail})
	} else if _, err := os.Stat(cfgPath); err != nil {
		checks = append(checks, diagCheck{"Config", "no config file (using defaults)", statusWarn})
	} else {
		checks = append(checks, diagCheck{"Config", cfgPath, statusOK})
	}

	// Effective isolation mode.
	checks = append(checks, diagCheck{"Isolation mode", cfg.IsolationFor("", ""), statusOK})

	// Container binary resolution (macOS-oriented; nerdctl on Linux).
	if runtime.GOOS == "darwin" {
		path, source := engine.ResolveBinaryInfo(cfg.RuntimeBinary())
		if _, err := os.Stat(path); err == nil {
			checks = append(checks, diagCheck{"Container binary", fmt.Sprintf("%s (via %s)", path, source), statusOK})
		} else {
			checks = append(checks, diagCheck{"Container binary", fmt.Sprintf("%s not found (via %s) — run 'gocker setup' or set runtimeBinary in config", path, source), statusFail})
		}
	}

	// Daemon socket/pid health. fsutil.HomeDir os.Exit(1)s if $HOME can't be
	// resolved — acceptable here since doctor is only useful with a real home dir.
	dir := filepath.Join(fsutil.HomeDir(), ".gocker")
	sock := filepath.Join(dir, "gocker.sock")
	if _, err := os.Stat(sock); err == nil {
		checks = append(checks, diagCheck{"Daemon socket", sock + " (present)", statusOK})
	} else {
		checks = append(checks, diagCheck{"Daemon socket", "not running (start with 'gocker daemon start')", statusWarn})
	}

	return checks
}
