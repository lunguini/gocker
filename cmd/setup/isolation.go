// Package setup provides the interactive wizard for configuring gocker.
package setup

import "fmt"

const (
	IsolationFull   = "full"
	IsolationHybrid = "hybrid"
	IsolationShared = "shared"
)

// IsolationChoices are the options offered in the wizard, in display order.
var IsolationChoices = []string{IsolationFull, IsolationHybrid, IsolationShared}

// PrintIsolationExplanations prints a short explanation of each mode before
// the prompt. Kept short so users can actually read it.
func PrintIsolationExplanations() {
	fmt.Println(`
Isolation modes determine how containers are separated:

  full    — every container gets its own lightweight Linux VM.
            Strongest isolation (hardware VM boundary per container).
            Heaviest: each 'gocker run' boots a fresh VM.
            Compose containers live inside per-project VMs and are not
            visible to 'gocker ps' — you must use 'gocker compose' commands.

  hybrid  — compose/run share one VM, sandboxes get dedicated VMs.
            Balanced: fast compose startup, strong sandbox isolation.

  shared  — everything runs in a single persistent VM (Docker-like).
            Fastest and most familiar. Standard namespace/cgroup isolation
            between containers, not hardware VM boundaries.
            Recommended default for development and CI.`)
}

// ChooseIsolation runs the interactive prompt (or returns the default in
// non-interactive mode).
func ChooseIsolation(interactive bool, defaultMode string) string {
	if !interactive {
		return defaultMode
	}
	PrintIsolationExplanations()
	return Choose("\nSelect isolation mode:", IsolationChoices, defaultMode)
}
