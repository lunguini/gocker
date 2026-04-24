package setup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lunguini/gocker/config"
)

// Options controls wizard behavior.
type Options struct {
	NonInteractive bool // --yes: use CI-friendly defaults, skip shell/docker prompts
}

// RunWizard executes the interactive (or silent) configuration wizard.
// Called after install steps in 'gocker setup'.
func RunWizard(ctx context.Context, opts Options) error {
	interactive := !opts.NonInteractive && IsInteractive()
	// Earlier install steps shell out to `container system start` in
	// interactive mode. Even with save/restore around those calls, some
	// paths (child crashing, signal delivery) leave the terminal in raw
	// mode — Enter then produces CR not LF and our prompts hang forever.
	// stty sane is the belt-and-suspenders fix; no-op when stdin isn't a TTY.
	if interactive {
		NormalizeTerminal()
		fmt.Println("Configure gocker. Press Enter to accept the default, or type 'q' / 'quit' / Esc+Enter to cancel.")
	}
	existing := config.Load()

	// Isolation.
	defaultIso := existing.Isolation
	if defaultIso == "" {
		defaultIso = IsolationShared // suggest shared for new users and CI
	}
	isolation := ChooseIsolation(interactive, defaultIso)

	// Resources.
	cpu, mem := existing.SharedVM.CPUs, existing.SharedVM.Memory
	if cpu == 0 || mem == "" || isolation != existing.Isolation {
		hostCPUs, hostMemGB := detectHostResources()
		cpu, mem = defaultResources(isolation, hostCPUs, hostMemGB)
	}
	if interactive {
		cpu, mem = promptResources(cpu, mem)
	}

	// Persist config.
	cfg := existing
	cfg.Isolation = isolation
	cfg.SharedVM.CPUs = cpu
	cfg.SharedVM.Memory = mem
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Saved ~/.gocker/config.yaml (isolation=%s, cpus=%d, memory=%s)\n", isolation, cpu, mem)

	// Shell integration and docker context are skipped under --yes and in
	// non-interactive environments (don't silently modify dotfiles in CI).
	if !interactive {
		return nil
	}

	home, _ := os.UserHomeDir()
	socket := home + "/.gocker/gocker.sock"

	if Confirm("Add DOCKER_HOST + testcontainers env vars to your shell rc?", true) {
		shell := DetectShell(os.Getenv("SHELL"))
		if shell == "" {
			fmt.Printf("Unsupported shell (%s); skipping shell integration.\n", os.Getenv("SHELL"))
		} else {
			rc := ShellRCPath(shell, home)
			changed, err := InstallShellBlock(rc, shell, socket)
			if err != nil {
				fmt.Printf("Shell integration failed: %v\n", err)
			} else if changed {
				fmt.Printf("Updated %s. Reload your shell to pick up the new env vars.\n", rc)
			} else {
				fmt.Printf("%s already points at gocker — no changes needed.\n", rc)
			}
		}
	}

	if Confirm("Set 'gocker' as the active docker context?", true) {
		changed, err := ConfigureDockerContext(ctx, socket)
		if err != nil {
			fmt.Printf("Docker context setup failed: %v\n", err)
		} else if changed {
			fmt.Println("Active docker context is now 'gocker'.")
		} else {
			fmt.Println("Docker context already set — no changes needed.")
		}
	}

	return nil
}

func promptResources(defCPU int, defMem string) (int, string) {
	cpuStr := Input("VM CPUs", fmt.Sprintf("%d", defCPU))
	var cpu int
	if _, err := fmt.Sscanf(cpuStr, "%d", &cpu); err != nil || cpu < 1 {
		cpu = defCPU
	}
	mem := Input("VM memory (with K/M/G/T suffix, e.g. 4G)", defMem)
	mem = normalizeMemory(mem, defMem)
	return cpu, mem
}

// normalizeMemory ensures the memory string carries a unit suffix. Apple's
// container CLI interprets a plain integer as *bytes*, so "4" becomes 4 bytes
// and fails with "minimum memory amount allowed is 200 MiB". Users who type
// "4" almost always mean "4G", so assume G when no suffix is given.
func normalizeMemory(mem, fallback string) string {
	mem = strings.TrimSpace(mem)
	if mem == "" {
		return fallback
	}
	last := mem[len(mem)-1]
	switch last {
	case 'K', 'k', 'M', 'm', 'G', 'g', 'T', 't', 'P', 'p', 'B', 'b':
		return mem
	}
	if _, err := strconv.Atoi(mem); err == nil {
		return mem + "G"
	}
	return mem
}

