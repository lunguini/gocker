package sharedvm

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lunguini/gocker/engine"
)

// SharedVMRuntime implements engine.Runtime by proxying all operations
// into a persistent shared VM. Each method constructs a gocker CLI command
// and executes it via `container exec gocker-shared gocker <args>`.
type SharedVMRuntime struct {
	manager *Manager
	apple   engine.Runtime // Apple Engine for exec-ing into the VM
}

func NewSharedVMRuntime(manager *Manager, apple engine.Runtime) *SharedVMRuntime {
	return &SharedVMRuntime{manager: manager, apple: apple}
}

func (s *SharedVMRuntime) BinaryPath() string {
	return s.apple.BinaryPath()
}

// Validate checks that the Apple runtime exists. The VM itself is created lazily.
func (s *SharedVMRuntime) Validate() error {
	return s.apple.Validate()
}

// --- Low-level exec (proxy raw args as gocker commands) ---

func (s *SharedVMRuntime) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, nil, err
	}
	vmArgs := s.proxyArgs(false, args...)
	return s.apple.Exec(ctx, vmArgs...)
}

func (s *SharedVMRuntime) ExecInteractive(ctx context.Context, args ...string) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	vmArgs := s.proxyArgs(true, args...)
	return s.apple.ExecInteractive(ctx, vmArgs...)
}

func (s *SharedVMRuntime) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, args...)
	return s.apple.ExecStream(ctx, vmArgs...)
}

// --- Container lifecycle ---

func (s *SharedVMRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	translated := s.translateMountArgs(args)
	gockerArgs := append([]string{"run"}, translated...)
	vmArgs := s.proxyArgs(interactive, gockerArgs...)
	if interactive {
		return s.apple.ExecInteractive(ctx, vmArgs...)
	}
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
	}
	return nil
}

func (s *SharedVMRuntime) ContainerList(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	args := []string{"ps", "--format", "json"}
	if all {
		args = append(args, "-a")
	}
	vmArgs := s.proxyArgs(false, args...)
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	// Output is from gocker inside VM which uses nerdctl — parse as nerdctl format
	return engine.ParseNerdctlContainerList(stdout)
}

func (s *SharedVMRuntime) ContainerStop(ctx context.Context, nameOrID string) error {
	return s.proxySimple(ctx, "stop", nameOrID)
}

func (s *SharedVMRuntime) ContainerStart(ctx context.Context, nameOrID string) error {
	return s.proxySimple(ctx, "start", nameOrID)
}

func (s *SharedVMRuntime) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	if force {
		_ = s.ContainerStop(ctx, nameOrID)
	}
	return s.proxySimple(ctx, "rm", nameOrID)
}

func (s *SharedVMRuntime) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	gockerArgs := []string{"exec"}
	if interactive {
		gockerArgs = append(gockerArgs, "-it")
	}
	gockerArgs = append(gockerArgs, nameOrID)
	gockerArgs = append(gockerArgs, args...)
	vmArgs := s.proxyArgs(interactive, gockerArgs...)
	if interactive {
		return s.apple.ExecInteractive(ctx, vmArgs...)
	}
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
	}
	return nil
}

func (s *SharedVMRuntime) ContainerLogs(ctx context.Context, nameOrID string, follow bool) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	gockerArgs := []string{"logs", nameOrID}
	if follow {
		gockerArgs = append(gockerArgs, "--follow")
		vmArgs := s.proxyArgs(true, gockerArgs...)
		return s.apple.ExecInteractive(ctx, vmArgs...)
	}
	vmArgs := s.proxyArgs(false, gockerArgs...)
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := string(stdout) + string(stderr)
	if out != "" {
		fmt.Print(out)
	}
	return nil
}

func (s *SharedVMRuntime) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, "inspect", nameOrID)
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// --- Image operations ---

func (s *SharedVMRuntime) ImagePull(ctx context.Context, image string) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	vmArgs := s.proxyArgs(true, "pull", image)
	return s.apple.ExecInteractive(ctx, vmArgs...)
}

func (s *SharedVMRuntime) ImagePush(ctx context.Context, image string) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	vmArgs := s.proxyArgs(true, "push", image)
	return s.apple.ExecInteractive(ctx, vmArgs...)
}

func (s *SharedVMRuntime) ImageList(ctx context.Context) ([]engine.ImageInfo, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, "images", "--format", "json")
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return engine.ParseNerdctlImageList(stdout)
}

func (s *SharedVMRuntime) ImageRemove(ctx context.Context, image string) error {
	return s.proxySimple(ctx, "rmi", image)
}

func (s *SharedVMRuntime) ImageBuild(ctx context.Context, args []string) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	translated := s.translateMountArgs(args)
	gockerArgs := append([]string{"build"}, translated...)
	vmArgs := s.proxyArgs(true, gockerArgs...)
	return s.apple.ExecInteractive(ctx, vmArgs...)
}

// --- Network operations ---

func (s *SharedVMRuntime) NetworkCreate(ctx context.Context, name string) error {
	return s.proxySimple(ctx, "network", "create", name)
}

func (s *SharedVMRuntime) NetworkList(ctx context.Context) ([]engine.NetworkInfo, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, "network", "ls", "--format", "json")
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return engine.ParseNerdctlNetworkList(stdout)
}

func (s *SharedVMRuntime) NetworkRemove(ctx context.Context, name string) error {
	return s.proxySimple(ctx, "network", "rm", name)
}

func (s *SharedVMRuntime) NetworkConnect(ctx context.Context, network, container string) error {
	return s.proxySimple(ctx, "network", "connect", network, container)
}

func (s *SharedVMRuntime) NetworkDisconnect(ctx context.Context, network, container string) error {
	return s.proxySimple(ctx, "network", "disconnect", network, container)
}

func (s *SharedVMRuntime) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, "network", "inspect", name)
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// --- Volume operations ---

func (s *SharedVMRuntime) VolumeCreate(ctx context.Context, name string) error {
	return s.proxySimple(ctx, "volume", "create", name)
}

func (s *SharedVMRuntime) VolumeList(ctx context.Context) ([]engine.VolumeInfo, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, "volume", "ls", "--format", "json")
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return engine.ParseNerdctlVolumeList(stdout)
}

func (s *SharedVMRuntime) VolumeRemove(ctx context.Context, name string) error {
	return s.proxySimple(ctx, "volume", "rm", name)
}

func (s *SharedVMRuntime) VolumeInspect(ctx context.Context, name string) ([]byte, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	vmArgs := s.proxyArgs(false, "volume", "inspect", name)
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// --- Helpers ---

// proxyArgs builds the full arg list for proxying a gocker command into the shared VM.
// Result: ["exec", [-it], "gocker-shared", "gocker", ...gockerArgs]
func (s *SharedVMRuntime) proxyArgs(interactive bool, gockerArgs ...string) []string {
	args := []string{"exec"}
	if interactive {
		args = append(args, "-i", "-t")
	}
	args = append(args, vmName, "gocker")
	args = append(args, gockerArgs...)
	return args
}

// proxySimple runs a simple gocker command in the VM and returns any error.
func (s *SharedVMRuntime) proxySimple(ctx context.Context, gockerArgs ...string) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	vmArgs := s.proxyArgs(false, gockerArgs...)
	_, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

// translateMountArgs scans args for -v/--volume flags and translates host paths
// to VM-internal paths.
func (s *SharedVMRuntime) translateMountArgs(args []string) []string {
	result := make([]string, len(args))
	copy(result, args)
	for i := 0; i < len(result)-1; i++ {
		if result[i] == "-v" || result[i] == "--volume" {
			result[i+1] = TranslateVolumeSpec(result[i+1], s.manager.Mounts())
		}
	}
	return result
}

// Compile-time check.
var _ engine.Runtime = (*SharedVMRuntime)(nil)
