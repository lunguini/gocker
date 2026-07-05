package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/lunguini/gocker/engine"
)

// validSandboxName matches safe sandbox names: no path separators or
// traversal sequences, so a hostile --name can't escape ~/.gocker/sandboxes/
// when building state file paths.
var validSandboxName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateName rejects sandbox names that could escape the sandbox state
// directory (e.g. "../../foo") when used to build a state file path.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("sandbox name must not be empty")
	}
	if !validSandboxName.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid sandbox name %q: only letters, digits, '.', '_', and '-' are allowed", name)
	}
	return nil
}

type Manager struct {
	eng engine.Runtime
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
	SyncSession     bool
	ManagedSettings bool
}

func NewManager(eng engine.Runtime) *Manager {
	return &Manager{eng: eng}
}

func (m *Manager) Run(ctx context.Context, opts RunOptions) error {
	if err := ValidateName(opts.Name); err != nil {
		return err
	}

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
			_ = SaveState(existing)
			if !opts.Detach {
				return m.Attach(ctx, opts.Name)
			}
			return nil
		default:
			// Container gone — clean up stale state and recreate
			fmt.Printf("Sandbox %q has stale state, recreating...\n", opts.Name)
			_ = DeleteState(opts.Name)
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
		return fmt.Errorf("unknown agent %q (available: claude, custom)", opts.Agent)
	}

	if image == "" {
		return fmt.Errorf("--image is required for custom agent")
	}

	if opts.EntryOverride != "" {
		entryCmd = strings.Fields(opts.EntryOverride)
	}

	// Build run args
	var args []string
	if !opts.Detach {
		args = append(args, "-i")
		if engine.IsTerminal() {
			args = append(args, "-t")
		}
	}
	if opts.Detach {
		args = append(args, "-d")
	}
	args = append(args, "--name", opts.Name)
	args = append(args, "-m", "4G")
	args = append(args, "-v", opts.Workspace+":/workspace")
	args = append(args, "-w", "/workspace")

	// Always forward TERM so TUI apps render correctly. Not a secret, so a
	// plain -e is fine — it's argv either way and TERM values aren't
	// sensitive.
	if term := os.Getenv("TERM"); term != "" {
		args = append(args, "-e", "TERM="+term)
	}

	// Required env vars from host (e.g. ANTHROPIC_API_KEY) and user-supplied
	// --env values often carry secrets. Apple's `container run --env-file`
	// reads a KEY=VALUE file instead of putting them on argv, where they'd
	// otherwise be visible to every local process via `ps aux` for the
	// lifetime of this command (C7b). Write them to a 0600 temp file instead.
	var secretEnvLines []string
	if tmpl != nil {
		for _, envName := range tmpl.EnvVars {
			if val := os.Getenv(envName); val != "" {
				secretEnvLines = append(secretEnvLines, envName+"="+val)
			}
		}
	}
	secretEnvLines = append(secretEnvLines, opts.ExtraEnv...)

	if len(secretEnvLines) > 0 {
		envFile, err := writeSecretEnvFile(secretEnvLines)
		if err != nil {
			return fmt.Errorf("writing sandbox env file: %w", err)
		}
		defer func() { _ = os.Remove(envFile) }()
		args = append(args, "--env-file", envFile)
	}

	// Config sync mounts
	configMounts := GetConfigMounts(opts.Agent, opts.SyncConfig, opts.SyncState, opts.ManagedSettings)
	args = append(args, GenerateMountFlags(configMounts)...)

	// Session sync — mount host session dir so /resume works across host and sandbox
	if opts.SyncSession {
		sessionMounts := SessionSyncMounts(opts.Workspace, "/workspace")
		args = append(args, GenerateMountFlags(sessionMounts)...)
	}

	// Image and entry command
	args = append(args, image)
	args = append(args, entryCmd...)

	// Run the container
	interactive := !opts.Detach && engine.IsTerminal()
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
	_ = SaveState(state)

	fmt.Printf("Sandbox %q created\n", opts.Name)
	return nil
}

// writeSecretEnvFile writes KEY=VALUE lines to a 0600 temp file for
// `container run --env-file`, keeping secrets off argv (ps aux visibility).
// Callers are responsible for removing the returned path once the backend
// CLI has read it (it's read synchronously during `container run`, so it's
// safe to remove as soon as ContainerRun returns, detached or not).
func writeSecretEnvFile(lines []string) (string, error) {
	f, err := os.CreateTemp("", "gocker-sandbox-env-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// getContainerStatus checks the real container status via inspect.
// Returns "running", "stopped", or "" if the container doesn't exist.
//
// Apple's inspect output may be a JSON array or a single object depending on
// backend/version (see sharedvm.Manager.getContainerStatus for the same
// tolerant handling) — treating it as object-only here caused a live
// sandbox to be misread as gone and force-recreated (H4).
func (m *Manager) getContainerStatus(ctx context.Context, nameOrID string) string {
	data, err := m.eng.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return ""
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		var arr []map[string]any
		if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
			raw = arr[0]
		}
	}
	if status, ok := raw["status"].(string); ok {
		return status
	}
	// Try nested format (e.g. Apple's "configuration"/"state" wrappers).
	s := string(data)
	for _, candidate := range []string{`"status":"`, `"Status":"`} {
		if idx := strings.Index(s, candidate); idx != -1 {
			start := idx + len(candidate)
			end := strings.Index(s[start:], `"`)
			if end != -1 {
				return s[start : start+end]
			}
		}
	}
	return ""
}

func (m *Manager) List() ([]*SandboxState, error) {
	return ListStates()
}

// ListLive returns sandbox states with Status refreshed against a live
// container inspect, instead of trusting the last-known value written to
// disk. `sandbox ls` previously reported "running" forever once a sandbox
// exited outside gocker's control (crash, `container stop`, host reboot);
// this keeps the on-disk state untouched (no write here) and just corrects
// what's displayed.
func (m *Manager) ListLive(ctx context.Context) ([]*SandboxState, error) {
	states, err := ListStates()
	if err != nil {
		return nil, err
	}
	for _, s := range states {
		if live := m.getContainerStatus(ctx, s.ContainerID); live != "" {
			s.Status = live
		} else {
			s.Status = "gone"
		}
	}
	return states, nil
}

func (m *Manager) Stop(ctx context.Context, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
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
	if err := ValidateName(name); err != nil {
		return err
	}
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
	if err := ValidateName(name); err != nil {
		return err
	}
	state, err := LoadState(name)
	if err != nil {
		return fmt.Errorf("sandbox %q not found", name)
	}
	// Apple's container CLI has no "attach" command.
	// Use "exec" with an interactive shell instead. Launch via /bin/sh
	// (present in virtually every image) and upgrade to bash when
	// available, so bash-less images don't fail with a cryptic exec error.
	shell := []string{"/bin/sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}
	return m.eng.ContainerExec(ctx, state.ContainerID, shell, true)
}

func (m *Manager) Logs(ctx context.Context, name string, follow bool) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	state, err := LoadState(name)
	if err != nil {
		return fmt.Errorf("sandbox %q not found", name)
	}
	return m.eng.ContainerLogs(ctx, state.ContainerID, engine.LogsOptions{Follow: follow})
}
