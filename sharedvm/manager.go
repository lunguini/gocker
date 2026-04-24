package sharedvm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
)

// Manager handles the lifecycle of the persistent shared VM.
type Manager struct {
	apple  engine.Runtime
	config config.SharedVM
	mounts map[string]string
	name   string // VM container name
}

func NewManager(apple engine.Runtime, cfg config.SharedVM) *Manager {
	mounts := DefaultMounts(cfg.EffectiveWorkspaceDirs())
	return &Manager{
		apple:  apple,
		config: cfg,
		mounts: mounts,
		name:   vmName,
	}
}

// NewManagerWithName creates a manager for a VM with a custom name.
// Used for per-project compose VMs in full isolation mode.
func NewManagerWithName(apple engine.Runtime, cfg config.SharedVM, name string) *Manager {
	mounts := DefaultMounts(cfg.EffectiveWorkspaceDirs())
	return &Manager{
		apple:  apple,
		config: cfg,
		mounts: mounts,
		name:   name,
	}
}

// Name returns the VM container name.
func (m *Manager) Name() string {
	return m.name
}

// Mounts returns the host→VM path mappings.
func (m *Manager) Mounts() map[string]string {
	return m.mounts
}

// EnsureRunningIfExists starts the VM if it exists but is stopped, and
// returns (true, nil) if the VM is running after the call. If the VM does
// not exist, it returns (false, nil) *without* creating it. Use this for
// read-only operations (list, prune) that have nothing to do when the VM
// has never been created.
func (m *Manager) EnsureRunningIfExists(ctx context.Context) (bool, error) {
	status := m.getContainerStatus(ctx)
	switch status {
	case "running":
		return true, nil
	case "stopped":
		fmt.Fprintln(os.Stderr, "Starting shared VM...")
		if err := m.apple.ContainerStart(ctx, m.name); err != nil {
			return false, fmt.Errorf("starting shared VM: %w", err)
		}
		m.updateState("running")
		return true, nil
	}
	return false, nil
}

// EnsureRunning ensures the shared VM is running, creating it if needed.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	status := m.getContainerStatus(ctx)
	switch status {
	case "running":
		return nil
	case "stopped":
		fmt.Fprintln(os.Stderr, "Starting shared VM...")
		if err := m.apple.ContainerStart(ctx, m.name); err != nil {
			return fmt.Errorf("starting shared VM: %w", err)
		}
		m.updateState("running")
		return nil
	}

	// VM doesn't exist — create it.
	// Double-check with a direct exec probe before destroying anything,
	// in case inspect/parse failed but the VM is actually alive.
	if _, _, probeErr := m.apple.Exec(ctx, "exec", m.name, "true"); probeErr == nil {
		m.updateState("running")
		return nil
	}

	fmt.Fprintln(os.Stderr, "Creating shared VM...")

	// Clean up any orphaned VM
	_ = m.apple.ContainerRemove(ctx, m.name, true)

	args := m.buildCreateArgs()
	if err := m.apple.ContainerRun(ctx, args, false); err != nil {
		_ = m.apple.ContainerRemove(ctx, m.name, true)
		return fmt.Errorf("creating shared VM: %w", err)
	}

	// Wait for the VM to be ready — the init script needs time to start
	// containerd (and the gocker daemon) before we can exec into it.
	ready := false
	for range 30 {
		if _, _, err := m.apple.Exec(ctx, "exec", m.name, "true"); err == nil {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	if !ready {
		_ = m.apple.ContainerRemove(ctx, m.name, true)
		return fmt.Errorf("shared VM created but not responding — try again")
	}

	state := &VMState{
		Name:    m.name,
		Status:  "running",
		Image:   m.config.Image,
		Created: time.Now(),
		Mounts:  m.mounts,
	}
	_ = SaveVMState(state)

	fmt.Fprintln(os.Stderr, "Shared VM is ready")
	return nil
}

func (m *Manager) buildCreateArgs() []string {
	var args []string
	args = append(args, "-d")
	args = append(args, "--name", m.name)

	memory := normalizeMemory(m.config.Memory)
	args = append(args, "-m", memory)

	if m.config.CPUs > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", m.config.CPUs))
	}

	// Mount workspace directories
	args = append(args, MountFlags(m.mounts)...)

	// Image
	image := m.config.Image
	if image == "" {
		image = "ghcr.io/lunguini/gocker-base:latest"
	}
	args = append(args, image)

	return args
}

// Stop stops the shared VM.
func (m *Manager) Stop(ctx context.Context) error {
	if err := m.apple.ContainerStop(ctx, m.name); err != nil {
		return fmt.Errorf("stopping shared VM: %w", err)
	}
	m.updateState("stopped")
	fmt.Println("Shared VM stopped")
	return nil
}

// Remove force-removes the shared VM and cleans up state.
func (m *Manager) Remove(ctx context.Context) error {
	if err := m.apple.ContainerRemove(ctx, m.name, true); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	_ = DeleteVMState()
	fmt.Println("Shared VM removed")
	return nil
}

// Status returns the current VM status: "running", "stopped", or "".
func (m *Manager) Status(ctx context.Context) string {
	return m.getContainerStatus(ctx)
}

func (m *Manager) getContainerStatus(ctx context.Context) string {
	data, err := m.apple.ContainerInspect(ctx, m.name)
	if err != nil {
		// Inspect failed — could be transient. If state file says running,
		// probe with a lightweight exec to avoid nuking a healthy VM.
		if state, _ := LoadVMState(); state != nil && state.Status == "running" {
			if _, _, probeErr := m.apple.Exec(ctx, "exec", m.name, "true"); probeErr == nil {
				return "running"
			}
		}
		return ""
	}
	// Apple's inspect output may be a JSON array or a single object.
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
	// Try nested format
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
	return "unknown"
}

// VMIP returns the shared VM's IP address, or "" if unavailable.
func (m *Manager) VMIP(ctx context.Context) string {
	data, err := m.apple.ContainerInspect(ctx, m.name)
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
	if raw == nil {
		return ""
	}
	networks, _ := raw["networks"].([]any)
	if len(networks) == 0 {
		return ""
	}
	net, _ := networks[0].(map[string]any)
	if net == nil {
		return ""
	}
	ip, _ := net["ipv4Address"].(string)
	if idx := strings.Index(ip, "/"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// ExpandMounts adds new host paths to the VM's mount set.
// Since Apple Container only accepts -v at creation time, this requires
// stopping and recreating the VM. Returns an error if containers are
// running inside the VM.
func (m *Manager) ExpandMounts(ctx context.Context, paths []string) error {
	// Filter out paths already covered by existing mounts
	var needed []string
	for _, p := range paths {
		p = filepath.Clean(p)
		_, covered := TranslatePath(p, m.mounts)
		if !covered {
			needed = append(needed, p)
		}
	}
	if len(needed) == 0 {
		return nil
	}

	// Check for running containers inside the VM
	containers, err := m.listVMContainers(ctx)
	if err == nil && len(containers) > 0 {
		return fmt.Errorf("cannot expand mounts: containers are running in the shared VM (%d). Stop them first, or add paths to workspaceDirs in ~/.gocker/config.yaml", len(containers))
	}

	fmt.Fprintf(os.Stderr, "Recreating shared VM to add mount(s): %v\n", needed)

	// Add new mounts
	for _, p := range needed {
		m.mounts[p] = "/host" + p
	}

	// Stop and remove existing VM
	_ = m.apple.ContainerStop(ctx, m.name)
	_ = m.apple.ContainerRemove(ctx, m.name, true)

	// Recreate with expanded mounts
	args := m.buildCreateArgs()
	if err := m.apple.ContainerRun(ctx, args, false); err != nil {
		_ = m.apple.ContainerRemove(ctx, m.name, true)
		return fmt.Errorf("recreating shared VM with expanded mounts: %w", err)
	}

	// Wait for readiness
	ready := false
	for range 30 {
		if _, _, err := m.apple.Exec(ctx, "exec", m.name, "true"); err == nil {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	if !ready {
		_ = m.apple.ContainerRemove(ctx, m.name, true)
		return fmt.Errorf("shared VM recreated but not responding")
	}

	// Persist state with new mounts
	state := &VMState{
		Name:    m.name,
		Status:  "running",
		Image:   m.config.Image,
		Created: time.Now(),
		Mounts:  m.mounts,
	}
	_ = SaveVMState(state)

	fmt.Fprintln(os.Stderr, "Shared VM is ready")
	return nil
}

// listVMContainers lists containers running inside the shared VM.
func (m *Manager) listVMContainers(ctx context.Context) ([]engine.ContainerInfo, error) {
	return m.apple.ContainerList(ctx, true)
}

// normalizeMemory turns a user-supplied memory spec into something Apple's
// container CLI understands. A bare integer (e.g. "4") is treated as bytes by
// that CLI, which then fails with "minimum memory amount allowed is 200 MiB".
// Users almost always mean gigabytes, so append G in that case. Empty or "0"
// falls back to 4G.
func normalizeMemory(mem string) string {
	mem = strings.TrimSpace(mem)
	if mem == "" || mem == "0" {
		return "4G"
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

func (m *Manager) updateState(status string) {
	state, _ := LoadVMState()
	if state == nil {
		state = &VMState{Name: m.name, Image: m.config.Image, Created: time.Now()}
	}
	state.Status = status
	_ = SaveVMState(state)
}
