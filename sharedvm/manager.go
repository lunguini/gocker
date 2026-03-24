package sharedvm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
}

func NewManager(apple engine.Runtime, cfg config.SharedVM) *Manager {
	mounts := DefaultMounts(cfg.EffectiveWorkspaceDirs())
	return &Manager{
		apple:  apple,
		config: cfg,
		mounts: mounts,
	}
}

// Mounts returns the host→VM path mappings.
func (m *Manager) Mounts() map[string]string {
	return m.mounts
}

// EnsureRunning ensures the shared VM is running, creating it if needed.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	status := m.getContainerStatus(ctx)
	switch status {
	case "running":
		return nil
	case "stopped":
		fmt.Fprintln(os.Stderr, "Starting shared VM...")
		if err := m.apple.ContainerStart(ctx, vmName); err != nil {
			return fmt.Errorf("starting shared VM: %w", err)
		}
		m.updateState("running")
		return nil
	}

	// VM doesn't exist — create it
	fmt.Fprintln(os.Stderr, "Creating shared VM...")

	// Clean up any orphaned VM
	_ = m.apple.ContainerRemove(ctx, vmName, true)

	args := m.buildCreateArgs()
	if err := m.apple.ContainerRun(ctx, args, false); err != nil {
		_ = m.apple.ContainerRemove(ctx, vmName, true)
		return fmt.Errorf("creating shared VM: %w", err)
	}

	state := &VMState{
		Name:    vmName,
		Status:  "running",
		Image:   m.config.Image,
		Created: time.Now(),
		Mounts:  m.mounts,
	}
	SaveVMState(state)

	fmt.Fprintln(os.Stderr, "Shared VM is ready")
	return nil
}

func (m *Manager) buildCreateArgs() []string {
	var args []string
	args = append(args, "-d")
	args = append(args, "--name", vmName)

	memory := m.config.Memory
	if memory == "" {
		memory = "4G"
	}
	args = append(args, "-m", memory)

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
	if err := m.apple.ContainerStop(ctx, vmName); err != nil {
		return fmt.Errorf("stopping shared VM: %w", err)
	}
	m.updateState("stopped")
	fmt.Println("Shared VM stopped")
	return nil
}

// Remove force-removes the shared VM and cleans up state.
func (m *Manager) Remove(ctx context.Context) error {
	if err := m.apple.ContainerRemove(ctx, vmName, true); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	DeleteVMState()
	fmt.Println("Shared VM removed")
	return nil
}

// Status returns the current VM status: "running", "stopped", or "".
func (m *Manager) Status(ctx context.Context) string {
	return m.getContainerStatus(ctx)
}

func (m *Manager) getContainerStatus(ctx context.Context) string {
	data, err := m.apple.ContainerInspect(ctx, vmName)
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

func (m *Manager) updateState(status string) {
	state, _ := LoadVMState()
	if state == nil {
		state = &VMState{Name: vmName, Image: m.config.Image, Created: time.Now()}
	}
	state.Status = status
	SaveVMState(state)
}
