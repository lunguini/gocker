package sharedvm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/internal/fsutil"
	"github.com/lunguini/gocker/internal/jsonx"
	"github.com/lunguini/gocker/internal/termx"
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
	var running bool
	err := fsutil.WithLock(lifecycleLockPath(m.name), func() error {
		var innerErr error
		running, innerErr = m.ensureRunningIfExistsLocked(ctx)
		return innerErr
	})
	return running, err
}

func (m *Manager) ensureRunningIfExistsLocked(ctx context.Context) (bool, error) {
	status := m.getContainerStatus(ctx)
	switch status {
	case "running":
		m.syncMountsFromVM(ctx)
		return true, nil
	case "stopped":
		fmt.Fprintln(os.Stderr, "Starting shared VM...")
		if err := m.apple.ContainerStart(ctx, m.name); err != nil {
			return false, fmt.Errorf("starting shared VM: %w", err)
		}
		m.updateState("running")
		m.syncMountsFromVM(ctx)
		return true, nil
	}
	return false, nil
}

// EnsureRunning ensures the shared VM is running, creating it if needed.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	return fsutil.WithLock(lifecycleLockPath(m.name), func() error {
		return m.ensureRunningLocked(ctx)
	})
}

func (m *Manager) ensureRunningLocked(ctx context.Context) error {
	status := m.getContainerStatus(ctx)
	switch status {
	case "running":
		m.syncMountsFromVM(ctx)
		return nil
	case "stopped":
		fmt.Fprintln(os.Stderr, "Starting shared VM...")
		if err := m.apple.ContainerStart(ctx, m.name); err != nil {
			return fmt.Errorf("starting shared VM: %w", err)
		}
		m.updateState("running")
		m.syncMountsFromVM(ctx)
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

	// Clean up any orphaned VM. Safe now that we hold the per-VM lifecycle
	// lock and have confirmed (fresh status + exec probe above) that no
	// healthy VM answers to this name — this can no longer race a concurrent
	// creator into deleting its healthy VM.
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
	return m.buildCreateArgsWith(m.mounts)
}

func (m *Manager) buildCreateArgsWith(mounts map[string]string) []string {
	var args []string
	args = append(args, "-d")
	args = append(args, "--name", m.name)

	memory := normalizeMemory(m.config.Memory)
	args = append(args, "-m", memory)

	if m.config.CPUs > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", m.config.CPUs))
	}

	// Mount workspace directories
	args = append(args, MountFlags(mounts)...)

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
	return fsutil.WithLock(lifecycleLockPath(m.name), func() error {
		if err := m.apple.ContainerRemove(ctx, m.name, true); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		_ = DeleteVMState()
		fmt.Println("Shared VM removed")
		return nil
	})
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
	if status := jsonx.InspectStatus(data); status != "" {
		return status
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
//
// Apple Container only accepts -v at creation time, so this stops and
// recreates the VM — which destroys everything inside it (running and stopped
// containers, images, named volumes, build cache). Because the blast radius is
// large and easy to trigger accidentally (a single `gocker run -v <new-path>`),
// recreation is gated on explicit consent when the VM holds any state:
//   - GOCKER_ASSUME_YES set (non-empty) → proceed without prompting;
//   - otherwise, if stdin is a TTY → prompt and proceed only on an explicit yes;
//   - otherwise (non-interactive, no override) → refuse with a clear message.
//
// m.mounts is only updated after a successful recreate, so a failed recreate
// never leaves the in-memory map claiming coverage the VM doesn't have.
func (m *Manager) ExpandMounts(ctx context.Context, paths []string) error {
	return fsutil.WithLock(lifecycleLockPath(m.name), func() error {
		return m.expandMountsLocked(ctx, paths)
	})
}

func (m *Manager) expandMountsLocked(ctx context.Context, paths []string) error {
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

	// Recreation destroys everything inside the VM. If it holds any state,
	// require consent before proceeding.
	if summary, hasState := m.vmStateSummary(ctx); hasState {
		if err := confirmDestructiveRecreate(summary); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "Recreating shared VM to add mount(s): %v\n", needed)

	// Compute the expanded mount set *without* mutating m.mounts yet — if the
	// recreate fails, the map must still reflect the surviving VM's coverage.
	newMounts := make(map[string]string, len(m.mounts)+len(needed))
	maps.Copy(newMounts, m.mounts)
	for _, p := range needed {
		newMounts[p] = "/host" + p
	}

	// Stop and remove existing VM
	_ = m.apple.ContainerStop(ctx, m.name)
	_ = m.apple.ContainerRemove(ctx, m.name, true)

	// Recreate with expanded mounts
	args := m.buildCreateArgsWith(newMounts)
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

	// Recreate succeeded — now it is safe to adopt the expanded mount set.
	m.mounts = newMounts

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

// vmStateSummary reports whether the shared VM currently holds any state that
// a recreate would destroy, along with a human-readable summary for the
// confirmation prompt. It counts running + stopped containers and images
// *inside* the VM via nerdctl (not Apple-host-level containers, which would
// include the shared VM itself). A probe failure is treated as "no state" so a
// dead/absent VM doesn't block expansion.
func (m *Manager) vmStateSummary(ctx context.Context) (string, bool) {
	running := m.countVMLines(ctx, "ps", "-q")
	stopped := max(m.countVMLines(ctx, "ps", "-a", "-q")-running, 0)
	images := m.countVMLines(ctx, "images", "-q")
	if running == 0 && stopped == 0 && images == 0 {
		return "", false
	}
	return fmt.Sprintf("%d running container(s), %d stopped container(s), %d image(s)", running, stopped, images), true
}

// countVMLines runs `nerdctl <args>` inside the VM and counts non-empty output
// lines. Returns 0 on any error (VM unreachable, command failed).
func (m *Manager) countVMLines(ctx context.Context, args ...string) int {
	execArgs := append([]string{"exec", m.name, "nerdctl"}, args...)
	stdout, _, err := m.apple.Exec(ctx, execArgs...)
	if err != nil {
		return 0
	}
	n := 0
	for line := range strings.SplitSeq(string(stdout), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// confirmDestructiveRecreate enforces the consent policy documented on
// ExpandMounts. summary describes what would be destroyed.
func confirmDestructiveRecreate(summary string) error {
	if v := strings.TrimSpace(os.Getenv("GOCKER_ASSUME_YES")); v != "" {
		return nil
	}
	msg := fmt.Sprintf("recreating the shared VM to add a bind mount will destroy everything inside it (%s)", summary)
	if !termx.StdinIsTTY() {
		return fmt.Errorf("%s. Refusing without confirmation: re-run in a terminal to confirm, set GOCKER_ASSUME_YES=1, or add the path to sharedVM.workspaceDirs in ~/.gocker/config.yaml", msg)
	}
	fmt.Fprintf(os.Stderr, "Warning: %s.\nProceed? [y/N]: ", msg)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("aborted: shared VM not recreated")
	}
}

// syncMountsFromVM reconciles the manager's in-memory mount map with what
// the VM container actually has. Needed because a VM created in an earlier
// session may have fewer mounts than the current config requests — without
// this, TranslatePath lies about coverage and commands fail with confusing
// "file not found" errors.
func (m *Manager) syncMountsFromVM(ctx context.Context) {
	data, err := m.apple.ContainerInspect(ctx, m.name)
	if err != nil {
		return
	}
	var raw []map[string]any
	if json.Unmarshal(data, &raw) != nil || len(raw) == 0 {
		var one map[string]any
		if json.Unmarshal(data, &one) != nil {
			return
		}
		raw = []map[string]any{one}
	}
	cfg, _ := raw[0]["configuration"].(map[string]any)
	if cfg == nil {
		return
	}
	mountsAny, _ := cfg["mounts"].([]any)
	actual := map[string]string{}
	for _, ma := range mountsAny {
		mm, _ := ma.(map[string]any)
		if mm == nil {
			continue
		}
		src, _ := mm["source"].(string)
		dst, _ := mm["destination"].(string)
		if src == "" || dst == "" {
			continue
		}
		src = filepath.Clean(src)
		actual[src] = dst
	}
	if len(actual) > 0 {
		m.mounts = actual
	}
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
