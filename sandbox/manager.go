package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lunguini/gocker/engine"
)

type Manager struct {
	eng *engine.Engine
}

type RunOptions struct {
	Name            string
	Agent           string
	Workspace       string
	NetworkPolicy   string
	AllowedHosts    []string
	ExtraEnv        []string
	ImageOverride   string
	EntryOverride   string
	Detach          bool
	SyncConfig      bool
	SyncState       bool
	ManagedSettings bool
}

func NewManager(eng *engine.Engine) *Manager {
	return &Manager{eng: eng}
}

func (m *Manager) Run(ctx context.Context, opts RunOptions) error {
	// Check if sandbox already exists in our state
	existing, err := LoadState(opts.Name)
	if err == nil {
		// Verify the container actually exists and check its real status
		realStatus := m.getContainerStatus(ctx, existing.ContainerID)
		switch realStatus {
		case "running":
			fmt.Printf("Sandbox %q is already running, reattaching...\n", opts.Name)
			return m.Attach(ctx, opts.Name)
		case "stopped":
			fmt.Printf("Sandbox %q exists but is stopped, starting...\n", opts.Name)
			if err := m.eng.ContainerStart(ctx, existing.ContainerID); err != nil {
				return fmt.Errorf("starting sandbox container: %w", err)
			}
			existing.Status = "running"
			SaveState(existing)
			if !opts.Detach {
				return m.Attach(ctx, opts.Name)
			}
			return nil
		default:
			// Container gone — clean up stale state and recreate
			fmt.Printf("Sandbox %q has stale state, recreating...\n", opts.Name)
			DeleteState(opts.Name)
		}
	}

	// Clean up any orphaned container from a previous failed run
	// (exists in container CLI registry but not in our state)
	_ = m.eng.ContainerRemove(ctx, opts.Name, true)

	// Get template and ensure image exists
	tmpl := GetTemplate(opts.Agent)
	image := opts.ImageOverride
	var entryCmd []string

	if tmpl != nil {
		if image == "" {
			image = tmpl.Image
		}
		entryCmd = tmpl.EntryCmd
	} else if opts.Agent != "custom" {
		return fmt.Errorf("unknown agent %q (available: claude, codex, custom)", opts.Agent)
	}

	if image == "" {
		return fmt.Errorf("--image is required for custom agent")
	}

	if opts.EntryOverride != "" {
		entryCmd = strings.Fields(opts.EntryOverride)
	}

	// Build run args
	var args []string
	if opts.Detach || !opts.Detach {
		// Always create with -it for sandbox
		args = append(args, "-i", "-t")
	}
	if opts.Detach {
		args = append(args, "-d")
	}
	args = append(args, "--name", opts.Name)
	args = append(args, "-m", "4G")
	args = append(args, "-v", opts.Workspace+":/workspace")
	args = append(args, "-w", "/workspace")

	// Always forward TERM so TUI apps render correctly
	if term := os.Getenv("TERM"); term != "" {
		args = append(args, "-e", "TERM="+term)
	}

	// Pass required env vars from host
	if tmpl != nil {
		for _, envName := range tmpl.EnvVars {
			if val := os.Getenv(envName); val != "" {
				args = append(args, "-e", envName+"="+val)
			}
		}
	}

	// Extra env vars
	for _, e := range opts.ExtraEnv {
		args = append(args, "-e", e)
	}

	// Config sync mounts
	configMounts := GetConfigMounts(opts.Agent, opts.SyncConfig, opts.SyncState, opts.ManagedSettings)
	args = append(args, GenerateMountFlags(configMounts)...)

	// Image and entry command
	args = append(args, image)
	args = append(args, entryCmd...)

	// Run the container
	interactive := !opts.Detach
	if err := m.eng.ContainerRun(ctx, args, interactive); err != nil {
		// Clean up orphaned container that may have been registered
		// by the container CLI before the error occurred
		_ = m.eng.ContainerRemove(ctx, opts.Name, true)
		return fmt.Errorf("running sandbox container: %w", err)
	}

	// Try to get container IP from inspect
	containerIP := ""
	inspectData, err := m.eng.ContainerInspect(ctx, opts.Name)
	if err == nil {
		var raw map[string]any
		if json.Unmarshal(inspectData, &raw) == nil {
			if ip, ok := raw["ip"].(string); ok {
				containerIP = ip
			}
		}
	}

	// Save state
	state := &SandboxState{
		Name:          opts.Name,
		Agent:         opts.Agent,
		Workspace:     opts.Workspace,
		ContainerID:   opts.Name, // Use name as ID for now
		Status:        "running",
		Created:       time.Now(),
		NetworkPolicy: opts.NetworkPolicy,
		AllowedHosts:  opts.AllowedHosts,
		ContainerIP:   containerIP,
	}
	SaveState(state)

	fmt.Printf("Sandbox %q created\n", opts.Name)
	return nil
}

// getContainerStatus checks the real container status via inspect.
// Returns "running", "stopped", or "" if the container doesn't exist.
func (m *Manager) getContainerStatus(ctx context.Context, nameOrID string) string {
	data, err := m.eng.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return ""
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return ""
	}
	if status, ok := raw["status"].(string); ok {
		return status
	}
	return ""
}

func (m *Manager) List() ([]*SandboxState, error) {
	return ListStates()
}

func (m *Manager) Stop(ctx context.Context, name string) error {
	state, err := LoadState(name)
	if err != nil {
		return fmt.Errorf("sandbox %q not found", name)
	}
	if err := m.eng.ContainerStop(ctx, state.ContainerID); err != nil {
		return err
	}
	state.Status = "stopped"
	return SaveState(state)
}

func (m *Manager) Remove(ctx context.Context, name string) error {
	state, err := LoadState(name)
	if err != nil {
		return fmt.Errorf("sandbox %q not found", name)
	}
	if err := m.eng.ContainerRemove(ctx, state.ContainerID, true); err != nil {
		// Don't fail if container already gone
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	return DeleteState(name)
}

func (m *Manager) Attach(ctx context.Context, name string) error {
	state, err := LoadState(name)
	if err != nil {
		return fmt.Errorf("sandbox %q not found", name)
	}
	// Apple's container CLI has no "attach" command.
	// Use "exec" with an interactive shell instead.
	return m.eng.ContainerExec(ctx, state.ContainerID, []string{"/bin/bash"}, true)
}

func (m *Manager) Logs(ctx context.Context, name string, follow bool) error {
	state, err := LoadState(name)
	if err != nil {
		return fmt.Errorf("sandbox %q not found", name)
	}
	return m.eng.ContainerLogs(ctx, state.ContainerID, follow)
}
