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

	"github.com/lunguini/gocker/internal/jsonx"
	"github.com/lunguini/gocker/internal/termx"
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

func (n *NerdctlRuntime) ExecStreamSplit(ctx context.Context, args ...string) (io.ReadCloser, io.ReadCloser, error) {
	return execStreamSplit(ctx, n.Binary, args...)
}

func (n *NerdctlRuntime) ExecStreamStdin(ctx context.Context, stdin io.Reader, args ...string) (io.ReadCloser, io.ReadCloser, error) {
	return execStreamSplitStdin(ctx, n.Binary, stdin, args...)
}

// --- Container operations ---

func (n *NerdctlRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	cmdArgs := append([]string{"run"}, args...)
	if interactive {
		return n.ExecInteractive(ctx, cmdArgs...)
	}
	stderr, err := execPassthrough(ctx, n.Binary, cmdArgs...)
	if err != nil {
		return wrapRunErr("nerdctl run", cmdArgs, nil, stderr, err)
	}
	return nil
}

// ContainerCreate runs `nerdctl create <args>` and returns the new
// container's ID (nerdctl prints it on stdout). No start — the API
// create/start split relies on this.
func (n *NerdctlRuntime) ContainerCreate(ctx context.Context, args []string) (string, error) {
	cmdArgs := append([]string{"create"}, args...)
	stdout, stderr, err := n.Exec(ctx, cmdArgs...)
	if err != nil {
		return "", wrapRunErr("nerdctl create", cmdArgs, stdout, stderr, err)
	}
	return strings.TrimSpace(string(stdout)), nil
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
	return &cliErr{msg: msg, cause: err, sentinel: classifySentinel(msg)}
}

// wrapNerdctlErr normalizes the stderr-less 'exit status 1' case for
// nerdctl shell-outs that don't have additional context to fall back on.
func wrapNerdctlErr(stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = "nerdctl produced no output"
	}
	return &cliErr{msg: msg, cause: err, sentinel: classifySentinel(msg)}
}

func (n *NerdctlRuntime) ContainerList(ctx context.Context, all bool) ([]ContainerInfo, error) {
	args := []string{"ps", "--format", "json"}
	if all {
		args = append(args, "-a")
	}
	stdout, stderr, err := n.Exec(ctx, args...)
	if err != nil {
		return nil, cliError(stderr, err)
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
			ID:      jsonx.GetString(obj, "ID", "id"),
			Name:    jsonx.GetString(obj, "Names", "Name", "name"),
			Image:   jsonx.GetString(obj, "Image", "image"),
			Status:  jsonx.GetString(obj, "Status", "status"),
			State:   jsonx.GetString(obj, "State", "state"),
			Command: jsonx.GetString(obj, "Command", "command"),
			Ports:   jsonx.GetString(obj, "Ports", "ports"),
			Labels:  parseNerdctlLabels(jsonx.GetString(obj, "Labels", "labels")),
		}
		if created := jsonx.GetString(obj, "CreatedAt", "Created", "created"); created != "" {
			if t, err := time.Parse(time.RFC3339, created); err == nil {
				info.Created = t
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// parseNerdctlLabels splits nerdctl's comma-separated Labels string into a
// map. The string is shaped like "k1=v1,k2=v2,..." but values can contain
// JSON fragments with embedded commas ({"a":1,"b":2} or [1,2]), so a naive
// comma-split breaks. Split only on commas that are at brace/bracket depth
// zero.
func parseNerdctlLabels(raw string) map[string]string {
	out := map[string]string{}
	if raw == "" {
		return out
	}
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, raw[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, raw[start:])
	for _, p := range parts {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok || k == "" {
			continue
		}
		out[k] = v
	}
	return out
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
		// -i always; -t only when stdin is a real terminal — nerdctl exec
		// rejects -t with "provided file is not a console" when stdin is a
		// pipe (e.g. `gocker exec -i c cat < file`, or any exec proxied
		// into the shared VM).
		cmdArgs = []string{"exec", "-i"}
		if termx.StdinIsTTY() {
			cmdArgs = append(cmdArgs, "-t")
		}
		cmdArgs = append(cmdArgs, nameOrID)
		cmdArgs = append(cmdArgs, args...)
		return n.ExecInteractive(ctx, cmdArgs...)
	}
	stderr, err := execPassthrough(ctx, n.Binary, cmdArgs...)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerLogs(ctx context.Context, nameOrID string, opts LogsOptions) error {
	args := []string{"logs"}
	args = append(args, LogsFlags(opts)...)
	args = append(args, nameOrID)
	if opts.Follow {
		return n.ExecInteractive(ctx, args...)
	}
	stderr, err := execPassthrough(ctx, n.Binary, args...)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	stdout, stderr, err := n.Exec(ctx, "inspect", nameOrID)
	if err != nil {
		return nil, cliError(stderr, err)
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
	if termx.StdoutIsTTY() {
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
		return nil, cliError(stderr, err)
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
		// nerdctl's `images --format json` emits both Repository (short:
		// "nginx") and Name (fully qualified: "docker.io/library/nginx:alpine").
		// Docker API clients and compose look up images by the fully
		// qualified form, so prefer Name — otherwise /images/{name}/json
		// returns 404 for perfectly pulled images and compose bails with
		// "image not found".
		size := jsonx.GetString(obj, "Size", "size")
		info := ImageInfo{
			ID:        jsonx.GetString(obj, "ID", "id"),
			Tag:       jsonx.GetString(obj, "Tag", "tag"),
			Size:      size,
			SizeBytes: parseSizeString(size),
		}
		if full := jsonx.GetString(obj, "Name", "name"); full != "" {
			// Strip the trailing :tag (if any) so Name holds just the repo
			// path and Tag stays consistent with the dedicated field.
			if i := strings.LastIndex(full, ":"); i > strings.LastIndex(full, "/") {
				info.Name = full[:i]
				if info.Tag == "" {
					info.Tag = full[i+1:]
				}
			} else {
				info.Name = full
			}
		}
		if info.Name == "" {
			info.Name = jsonx.GetString(obj, "Repository", "repository")
		}
		if info.ID == "" {
			info.ID = jsonx.GetString(obj, "Digest", "digest")
		}
		if created := jsonx.GetString(obj, "CreatedAt", "Created", "created"); created != "" {
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

func (n *NerdctlRuntime) NetworkCreate(ctx context.Context, name string, labels map[string]string) error {
	args := []string{"network", "create"}
	args = append(args, labelArgs(labels)...)
	args = append(args, name)
	_, stderr, err := n.Exec(ctx, args...)
	if err != nil {
		return wrapNerdctlErr(stderr, err)
	}
	return nil
}

func (n *NerdctlRuntime) NetworkList(ctx context.Context) ([]NetworkInfo, error) {
	stdout, stderr, err := n.Exec(ctx, "network", "ls", "--format", "json")
	if err != nil {
		return nil, cliError(stderr, err)
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
		name := jsonx.GetString(obj, "Name", "name")
		id := jsonx.GetString(obj, "ID", "id")
		if name == "" {
			name = id
		}
		if id == "" {
			id = name
		}
		result = append(result, NetworkInfo{
			ID:     id,
			Name:   name,
			Driver: jsonx.GetString(obj, "Driver", "driver"),
			Scope:  jsonx.GetString(obj, "Scope", "scope"),
			Labels: jsonx.ExtractLabelsFromAny(obj),
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
		return nil, cliError(stderr, err)
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
		return nil, cliError(stderr, err)
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
			Name:       jsonx.GetString(obj, "Name", "name"),
			Driver:     jsonx.GetString(obj, "Driver", "driver"),
			Mountpoint: jsonx.GetString(obj, "Mountpoint", "mountpoint"),
			Labels:     jsonx.ExtractLabelsFromAny(obj),
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
		return nil, cliError(stderr, err)
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
	for line := range strings.SplitSeq(trimmed, "\n") {
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
