package sharedvm

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"

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
	if bypass, ok := nerdctlBypass(args); ok {
		vmArgs = append([]string{"exec", s.manager.Name(), "nerdctl"}, bypass...)
	}
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
	if bypass, ok := nerdctlBypass(args); ok {
		vmArgs = append([]string{"exec", s.manager.Name(), "nerdctl"}, bypass...)
	}
	return s.apple.ExecStream(ctx, vmArgs...)
}

func (s *SharedVMRuntime) ExecStreamSplit(ctx context.Context, args ...string) (io.ReadCloser, io.ReadCloser, error) {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return nil, nil, err
	}
	vmArgs := s.proxyArgs(false, args...)
	if bypass, ok := nerdctlBypass(args); ok {
		vmArgs = append([]string{"exec", s.manager.Name(), "nerdctl"}, bypass...)
	}
	return s.apple.ExecStreamSplit(ctx, vmArgs...)
}

// nerdctlBypass returns the args to pass directly to nerdctl when the
// requested command would otherwise be routed through inner gocker.
// Used to avoid waiting on a new gocker-base image before exposing flags
// that nerdctl already supports natively.
func nerdctlBypass(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	switch args[0] {
	case "logs":
		return args, true
	}
	return nil, false
}

// --- Container lifecycle ---

func (s *SharedVMRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	translated, unmapped := s.translateMountArgs(args)
	if len(unmapped) > 0 {
		mountDirs, err := s.resolveUnmappedMounts(unmapped)
		if err != nil {
			return err
		}
		if err := s.manager.ExpandMounts(ctx, mountDirs); err != nil {
			return err
		}
		// Retry translation with expanded mounts
		translated, unmapped = s.translateMountArgs(args)
		if len(unmapped) > 0 {
			return fmt.Errorf("bind mount paths still not accessible after VM expansion: %v", unmapped)
		}
	}
	// Skip the intermediate `gocker run` inside the VM and shell out to
	// nerdctl directly. The inner gocker is mostly a pass-through for run
	// flags, and older base images reject newer flags the API layer emits
	// (--label is the most recent one). Bypassing avoids having to keep
	// the in-VM gocker in lockstep with every new flag we expose.
	runArgs := append([]string{"run"}, translated...)
	outer := []string{"exec"}
	if interactive && stdinIsTTY() {
		outer = append(outer, "-i", "-t")
	} else if interactive {
		outer = append(outer, "-i")
	}
	outer = append(outer, s.manager.Name(), "nerdctl")
	outer = append(outer, runArgs...)
	if interactive {
		return s.apple.ExecInteractive(ctx, outer...)
	}
	stdout, stderr, err := s.apple.Exec(ctx, outer...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
	}

	// If ports were published, tell the user how to reach them.
	if hasPortFlag(translated) {
		if ip := s.manager.VMIP(ctx); ip != "" {
			fmt.Fprintf(os.Stderr, "Ports are accessible via the shared VM at %s (not localhost)\n", ip)
		}
	}
	return nil
}

func (s *SharedVMRuntime) ContainerList(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
	running, err := s.manager.EnsureRunningIfExists(ctx)
	if err != nil {
		return nil, err
	}
	if !running {
		return nil, nil
	}
	// Go directly to nerdctl — the intermediate in-VM gocker flattens
	// Labels (among other fields) during its own parse/reprint cycle, so
	// labels set by the API ('docker compose up') never reach callers of
	// /containers/json. ParseNerdctlContainerList accepts nerdctl's JSON
	// shape natively.
	inner := []string{"ps", "--format", "json"}
	if all {
		inner = append(inner, "-a")
	}
	vmArgs := append([]string{"exec", s.manager.Name(), "nerdctl"}, inner...)
	stdout, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	// Output is from gocker inside VM which uses nerdctl — parse as nerdctl format
	containers, err2 := engine.ParseNerdctlContainerList(stdout)
	if err2 != nil {
		return nil, err2
	}

	// Rewrite 0.0.0.0 in port bindings to the VM's IP so the output is
	// directly usable from the host.
	if ip := s.manager.VMIP(ctx); ip != "" {
		for i := range containers {
			containers[i].Ports = strings.ReplaceAll(containers[i].Ports, "0.0.0.0:", ip+":")
		}
	}
	return containers, nil
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

func (s *SharedVMRuntime) ContainerLogs(ctx context.Context, nameOrID string, opts engine.LogsOptions) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	// Bypass inner gocker — older gocker-base images don't know the new
	// tail/since/until/timestamps flags. nerdctl accepts them natively.
	innerArgs := []string{"logs"}
	innerArgs = append(innerArgs, engine.LogsFlags(opts)...)
	innerArgs = append(innerArgs, nameOrID)
	outer := []string{"exec"}
	if opts.Follow && stdinIsTTY() {
		outer = append(outer, "-i", "-t")
	} else {
		outer = append(outer, "-i")
	}
	outer = append(outer, s.manager.Name(), "nerdctl")
	vmArgs := append(outer, innerArgs...)
	if opts.Follow {
		return s.apple.ExecInteractive(ctx, vmArgs...)
	}
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

func (s *SharedVMRuntime) ImagePull(ctx context.Context, image string, opts engine.ImagePullOpts) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	pullArgs := []string{"pull"}
	if opts.Platform != "" {
		pullArgs = append(pullArgs, "--platform", opts.Platform)
	}
	if opts.Progress != "" {
		pullArgs = append(pullArgs, "--progress", opts.Progress)
	}
	if opts.MaxConcurrent > 0 {
		pullArgs = append(pullArgs, "--max-concurrent-downloads", fmt.Sprintf("%d", opts.MaxConcurrent))
	}
	pullArgs = append(pullArgs, image)
	vmArgs := s.proxyArgs(true, pullArgs...)
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
	running, err := s.manager.EnsureRunningIfExists(ctx)
	if err != nil {
		return nil, err
	}
	if !running {
		return nil, nil
	}
	vmArgs := s.proxyArgs(false, "--format", "json", "images")
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
	translated, unmapped := s.translateMountArgs(args)
	if len(unmapped) > 0 {
		mountDirs, err := s.resolveUnmappedMounts(unmapped)
		if err != nil {
			return err
		}
		if err := s.manager.ExpandMounts(ctx, mountDirs); err != nil {
			return err
		}
		translated, unmapped = s.translateMountArgs(args)
		if len(unmapped) > 0 {
			return fmt.Errorf("build paths still not accessible after VM expansion: %v", unmapped)
		}
	}
	gockerArgs := append([]string{"build"}, translated...)
	vmArgs := s.proxyArgs(true, gockerArgs...)
	return s.apple.ExecInteractive(ctx, vmArgs...)
}

// --- Network operations ---

func (s *SharedVMRuntime) NetworkCreate(ctx context.Context, name string, labels map[string]string) error {
	if err := s.manager.EnsureRunning(ctx); err != nil {
		return err
	}
	// Bypass the intermediate in-VM gocker CLI and hit nerdctl directly —
	// older base-images don't recognize --label on `gocker network create`.
	// This keeps the label path working even before a fresh :base-dev rolls
	// out. Every other shared runtime op still goes through gocker because
	// they rely on its arg translation.
	inner := []string{"network", "create"}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		inner = append(inner, "--label", k+"="+labels[k])
	}
	inner = append(inner, name)
	vmArgs := append([]string{"exec", s.manager.Name(), "nerdctl"}, inner...)
	_, stderr, err := s.apple.Exec(ctx, vmArgs...)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (s *SharedVMRuntime) NetworkList(ctx context.Context) ([]engine.NetworkInfo, error) {
	running, err := s.manager.EnsureRunningIfExists(ctx)
	if err != nil {
		return nil, err
	}
	if !running {
		return nil, nil
	}
	vmArgs := s.proxyArgs(false, "--format", "json", "network", "ls")
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
	running, err := s.manager.EnsureRunningIfExists(ctx)
	if err != nil {
		return nil, err
	}
	if !running {
		return nil, nil
	}
	vmArgs := s.proxyArgs(false, "--format", "json", "volume", "ls")
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
// Result: ["exec", [-i|-it], "gocker-shared", "gocker", ...gockerArgs]
//
// Apple's `container exec -t` fails with POSIX errno 19 ("Operation not
// supported by device") when stdin isn't a real terminal — which is every
// API-daemon call, piped CLI call, or CI invocation. Add -t only when the
// caller actually has a TTY; otherwise -i alone carries interactive semantics
// (stdin open for streaming) without the PTY allocation that breaks.
func (s *SharedVMRuntime) proxyArgs(interactive bool, gockerArgs ...string) []string {
	args := []string{"exec"}
	if interactive {
		args = append(args, "-i")
		if stdinIsTTY() {
			args = append(args, "-t")
		}
	}
	args = append(args, s.manager.Name(), "gocker")
	args = append(args, gockerArgs...)
	return args
}

// stdinIsTTY reports whether stdin is an actual terminal. os.ModeCharDevice
// alone is insufficient — it also matches /dev/null and other character
// devices. Apple's `container exec -t` only works against a real pty.
func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
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

// hasPortFlag returns true if args contain -p or --publish.
func hasPortFlag(args []string) bool {
	for _, a := range args {
		if a == "-p" || a == "--publish" {
			return true
		}
	}
	return false
}

// resolveUnmappedMounts converts raw unmapped paths to mount directories
// using ResolveMountParent.
func (s *SharedVMRuntime) resolveUnmappedMounts(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var dirs []string
	for _, p := range paths {
		dir, err := ResolveMountParent(p)
		if err != nil {
			return nil, err
		}
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// translateMountArgs scans args for -v/--volume flags and translates host paths
// to VM-internal paths. Returns the translated args and any source paths that
// could not be translated (not covered by existing mounts).
func (s *SharedVMRuntime) translateMountArgs(args []string) ([]string, []string) {
	result := make([]string, len(args))
	copy(result, args)
	var unmapped []string
	for i := 0; i < len(result)-1; i++ {
		if result[i] == "-v" || result[i] == "--volume" {
			translated, err := TranslateVolumeSpec(result[i+1], s.manager.Mounts())
			if err != nil {
				// Extract the source path for mount expansion
				parts := strings.SplitN(result[i+1], ":", 2)
				unmapped = append(unmapped, parts[0])
			} else {
				result[i+1] = translated
			}
		}
	}
	return result, unmapped
}

// Compile-time check.
var _ engine.Runtime = (*SharedVMRuntime)(nil)
