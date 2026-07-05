// Package termx provides TTY detection shared across gocker's CLI, API
// server, and shared-VM proxy. It uses golang.org/x/term rather than
// os.ModeCharDevice, which also matches /dev/null and other character
// devices and would misjudge a redirected stream as interactive.
package termx

import (
	"os"

	"golang.org/x/term"
)

// StdinIsTTY reports whether stdin is connected to a real terminal.
func StdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// StdoutIsTTY reports whether stdout is connected to a real terminal.
func StdoutIsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// StderrIsTTY reports whether stderr is connected to a real terminal.
func StderrIsTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
