package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// NerdctlRuntime is the containerd/nerdctl backend (Linux).
type NerdctlRuntime struct {
	Binary string
}

func NewNerdctl(binary string) *NerdctlRuntime {
	if binary == "" {
		binary = "nerdctl"
	}
	return &NerdctlRuntime{Binary: binary}
}

func (n *NerdctlRuntime) BinaryPath() string {
	return n.Binary
}

func (n *NerdctlRuntime) Validate() error {
	_, err := exec.LookPath(n.Binary)
	if err != nil {
		return fmt.Errorf("nerdctl not found; install containerd and nerdctl to use gocker on Linux")
	}
	return nil
}

func (n *NerdctlRuntime) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, n.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (n *NerdctlRuntime) ExecInteractive(ctx context.Context, args ...string) error {
	oldState := saveTermState()
	if oldState != nil {
		defer restoreTermState(oldState)
	}

	cmd := exec.CommandContext(ctx, n.Binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (n *NerdctlRuntime) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, n.Binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return &streamReader{cmd: cmd, reader: stdout}, nil
}

// --- Container operations ---

func (n *NerdctlRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	cmdArgs := append([]string{"run"}, args...)
	if interactive {
		return n.ExecInteractive(ctx, cmdArgs...)
	}
	stdout, stderr, err := n.Exec(ctx, cmdArgs...)
	if err != nil {
		return wrapRunErr("nerdctl run", cmdArgs, stdout, stderr, err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
	}
	return nil
}

// wrapRunErr produces a useful error message when a shell-out fails. The
// previous formulation — fmt.Errorf("%s: %w", stderr, err) — degenerates to
// a bare ": exit status 1" when the underlying CLI writes nothing to stderr
// (which happens when it crashes, is killed, or writes to stdout instead).
// Clients like Docker Compose surface that as "Error response from daemon:
// exit status 1", hiding the actual failure. Fall back to stdout, then to
// the command line itself, so the API consumer always has something to go
// on.
func wrapRunErr(label string, args []string, stdout, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = strings.TrimSpace(string(stdout))
	}
	if msg == "" {
		msg = fmt.Sprintf("%s %s", label, strings.Join(args, " "))
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// wrapNerdctlErr normalizes the stderr-less 'exit status 1' case for
// nerdctl shell-outs that don't have additional context to fall back on.
func wrapNerdctlErr(stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = "nerdctl produced no output"
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func (n *NerdctlRuntime) ContainerList(ctx context.Context, all bool) ([]ContainerInfo, error) {
	args := []string{"ps", "--format", "json"}
	if all {
		args = append(args, "-a")
	}
	stdout, stderr, err := n.Exec(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return ParseNerdctlContainerList(stdout)
}

func ParseNerdctlContainerList(data []byte) ([]ContainerInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	// Collect raw JSON objects — try JSON array first, then newline-delimited
	objects := parseJSONObjects([]byte(trimmed))

	var result []ContainerInfo
	for _, obj := range objects {
		info := ContainerInfo{
			ID:      getString(obj, "ID", "id"),
			Name:    getString(obj, "Names", "Name", "name"),
			Image:   getString(obj, "Image", "image"),
			Status:  getString(obj, "Status", "status"),
			State:   getString(obj, "State", "state"),
			Command: getString(obj, "Command", "command"),
			Ports:   getString(obj, "Ports", "ports"),
		}
		if created := getString(obj, "CreatedAt", "Created", "created"); created != "" {
			if t, err := time.Parse(time.RFC3339, created); err == nil {
				info.Created = t
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func (n *NerdctlRuntime) ContainerStop(ctx context.Context, nameOrID string) error {
	_, stderr, err := n.Exec(ctx, "stop", nameOrID)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerStart(ctx context.Context, nameOrID string) error {
	_, stderr, err := n.Exec(ctx, "start", nameOrID)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, nameOrID)
	_, stderr, err := n.Exec(ctx, args...)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	cmdArgs := append([]string{"exec"}, nameOrID)
	cmdArgs = append(cmdArgs, args...)
	if interactive {
		// Prepend -it flags after "exec"
		cmdArgs = []string{"exec", "-it", nameOrID}
		cmdArgs = append(cmdArgs, args...)
		return n.ExecInteractive(ctx, cmdArgs...)
	}
	stdout, stderr, err := n.Exec(ctx, cmdArgs...)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	out := strings.TrimSpace(string(stdout))
	if out != "" {
		fmt.Println(out)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerLogs(ctx context.Context, nameOrID string, follow bool) error {
	args := []string{"logs", nameOrID}
	if follow {
		args = append(args, "-f")
		return n.ExecInteractive(ctx, args...)
	}
	stdout, stderr, err := n.Exec(ctx, args...)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	out := string(stdout) + string(stderr)
	if out != "" {
		fmt.Print(out)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	stdout, stderr, err := n.Exec(ctx, "inspect", nameOrID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// --- Image operations ---

func (n *NerdctlRuntime) ImagePull(ctx context.Context, image string, opts ImagePullOpts) error {
	args := []string{"pull"}
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	// nerdctl doesn't expose --max-concurrent-downloads at the CLI (containerd
	// daemon config) or a --progress flag; silently drop those opts.
	args = append(args, image)
	// Interactive for terminal users (shows progress); captured otherwise so
	// daemon/API callers can surface the real error instead of a bare
	// "exit status 1" with the stderr swallowed by /dev/null.
	if isStdoutTTY() {
		return n.ExecInteractive(ctx, args...)
	}
	stdout, stderr, err := n.Exec(ctx, args...)
	if err != nil {
		return wrapRunErr("nerdctl pull", args, stdout, stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ImagePush(ctx context.Context, image string) error {
	return n.ExecInteractive(ctx, "push", image)
}

func (n *NerdctlRuntime) ImageList(ctx context.Context) ([]ImageInfo, error) {
	stdout, stderr, err := n.Exec(ctx, "images", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return ParseNerdctlImageList(stdout)
}

func ParseNerdctlImageList(data []byte) ([]ImageInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	objects := parseJSONObjects([]byte(trimmed))

	var result []ImageInfo
	for _, obj := range objects {
		info := ImageInfo{
			ID:   getString(obj, "ID", "id"),
			Name: getString(obj, "Repository", "repository", "Name", "name"),
			Tag:  getString(obj, "Tag", "tag"),
			Size: getString(obj, "Size", "size"),
		}
		if info.ID == "" {
			info.ID = getString(obj, "Digest", "digest")
		}
		if created := getString(obj, "CreatedAt", "Created", "created"); created != "" {
			if t, err := time.Parse(time.RFC3339, created); err == nil {
				info.Created = t
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func (n *NerdctlRuntime) ImageRemove(ctx context.Context, image string) error {
	_, stderr, err := n.Exec(ctx, "rmi", image)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ImageBuild(ctx context.Context, args []string) error {
	cmdArgs := append([]string{"build"}, args...)
	return n.ExecInteractive(ctx, cmdArgs...)
}

// --- Network operations ---

func (n *NerdctlRuntime) NetworkCreate(ctx context.Context, name string) error {
	_, stderr, err := n.Exec(ctx, "network", "create", name)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) NetworkList(ctx context.Context) ([]NetworkInfo, error) {
	stdout, stderr, err := n.Exec(ctx, "network", "ls", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return ParseNerdctlNetworkList(stdout)
}

func ParseNerdctlNetworkList(data []byte) ([]NetworkInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	objects := parseJSONObjects([]byte(trimmed))

	var result []NetworkInfo
	for _, obj := range objects {
		// Same name-id fallback as the Apple parser: some backends populate
		// only one of the two. Never return an empty identifier — callers
		// will try to pass "" to network rm/prune and get opaque errors.
		name := getString(obj, "Name", "name")
		id := getString(obj, "ID", "id")
		if name == "" {
			name = id
		}
		if id == "" {
			id = name
		}
		result = append(result, NetworkInfo{
			ID:     id,
			Name:   name,
			Driver: getString(obj, "Driver", "driver"),
			Scope:  getString(obj, "Scope", "scope"),
			Labels: extractLabelsFromAny(obj),
		})
	}
	return result, nil
}

func (n *NerdctlRuntime) NetworkRemove(ctx context.Context, name string) error {
	_, stderr, err := n.Exec(ctx, "network", "rm", name)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) NetworkConnect(ctx context.Context, network, container string) error {
	_, stderr, err := n.Exec(ctx, "network", "connect", network, container)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) NetworkDisconnect(ctx context.Context, network, container string) error {
	_, stderr, err := n.Exec(ctx, "network", "disconnect", network, container)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	stdout, stderr, err := n.Exec(ctx, "network", "inspect", name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// --- Volume operations ---

func (n *NerdctlRuntime) VolumeCreate(ctx context.Context, name string) error {
	_, stderr, err := n.Exec(ctx, "volume", "create", name)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) VolumeList(ctx context.Context) ([]VolumeInfo, error) {
	stdout, stderr, err := n.Exec(ctx, "volume", "ls", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return ParseNerdctlVolumeList(stdout)
}

func ParseNerdctlVolumeList(data []byte) ([]VolumeInfo, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	objects := parseJSONObjects([]byte(trimmed))

	var result []VolumeInfo
	for _, obj := range objects {
		info := VolumeInfo{
			Name:       getString(obj, "Name", "name"),
			Driver:     getString(obj, "Driver", "driver"),
			Mountpoint: getString(obj, "Mountpoint", "mountpoint"),
			Labels:     extractLabelsFromAny(obj),
		}
		result = append(result, info)
	}
	return result, nil
}

func (n *NerdctlRuntime) VolumeRemove(ctx context.Context, name string) error {
	_, stderr, err := n.Exec(ctx, "volume", "rm", name)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) VolumeInspect(ctx context.Context, name string) ([]byte, error) {
	stdout, stderr, err := n.Exec(ctx, "volume", "inspect", name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// parseJSONObjects handles both JSON arrays and newline-delimited JSON objects.
// gocker's --format json outputs a JSON array; nerdctl outputs one JSON object per line.
func parseJSONObjects(data []byte) []map[string]any {
	trimmed := strings.TrimSpace(string(data))

	// Try JSON array first
	var arr []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		return arr
	}

	// Fall back to newline-delimited JSON objects
	var objects []map[string]any
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		objects = append(objects, obj)
	}
	return objects
}

// Compile-time check that NerdctlRuntime implements Runtime.
var _ Runtime = (*NerdctlRuntime)(nil)
