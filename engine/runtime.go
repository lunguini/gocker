package engine

import (
	"context"
	"io"
)

// Runtime is the interface that all container backends must implement.
// The current Engine struct (Apple Container CLI) implements this interface.
// Future backends (e.g., NerdctlRuntime) will also implement it.
type Runtime interface {
	// Validate checks that the backend binary exists and is usable.
	Validate() error

	// BinaryPath returns the path to the underlying runtime binary.
	BinaryPath() string

	// Low-level exec — used by API server and commands that need raw CLI access.
	Exec(ctx context.Context, args ...string) ([]byte, []byte, error)
	ExecInteractive(ctx context.Context, args ...string) error
	ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error)

	// Container lifecycle
	ContainerRun(ctx context.Context, args []string, interactive bool) error
	ContainerList(ctx context.Context, all bool) ([]ContainerInfo, error)
	ContainerStop(ctx context.Context, nameOrID string) error
	ContainerStart(ctx context.Context, nameOrID string) error
	ContainerRemove(ctx context.Context, nameOrID string, force bool) error
	ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error
	ContainerLogs(ctx context.Context, nameOrID string, opts LogsOptions) error
	ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error)

	// Images
	ImagePull(ctx context.Context, image string, opts ImagePullOpts) error
	ImagePush(ctx context.Context, image string) error
	ImageList(ctx context.Context) ([]ImageInfo, error)
	ImageRemove(ctx context.Context, image string) error
	ImageBuild(ctx context.Context, args []string) error

	// Networks. Labels may be nil. Compose v2 sends
	// com.docker.compose.{project,network,version}; without these, it
	// refuses to re-adopt the network on a subsequent 'docker compose up'.
	NetworkCreate(ctx context.Context, name string, labels map[string]string) error
	NetworkList(ctx context.Context) ([]NetworkInfo, error)
	NetworkRemove(ctx context.Context, name string) error
	NetworkConnect(ctx context.Context, network, container string) error
	NetworkDisconnect(ctx context.Context, network, container string) error
	NetworkInspect(ctx context.Context, name string) ([]byte, error)

	// Volumes
	VolumeCreate(ctx context.Context, name string) error
	VolumeList(ctx context.Context) ([]VolumeInfo, error)
	VolumeRemove(ctx context.Context, name string) error
	VolumeInspect(ctx context.Context, name string) ([]byte, error)
}

// LogsOptions controls container log retrieval. Zero value: full backlog, no follow.
type LogsOptions struct {
	Follow     bool
	Tail       string // "all" or a number; empty = backend default
	Since      string // RFC3339 timestamp or duration like "10m"
	Until      string // RFC3339 timestamp or duration
	Timestamps bool
}

// LogsFlags renders LogsOptions as CLI flags compatible with both Apple's
// `container logs` and `nerdctl logs`.
func LogsFlags(opts LogsOptions) []string {
	var args []string
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Tail != "" {
		args = append(args, "--tail", opts.Tail)
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "--until", opts.Until)
	}
	if opts.Timestamps {
		args = append(args, "--timestamps")
	}
	return args
}

// ImagePullOpts controls image pull behavior. The zero value uses backend defaults.
type ImagePullOpts struct {
	// Platform restricts the pull to a single platform, e.g. "linux/arm64".
	// Empty string uses the backend default (typically host architecture).
	Platform string
	// MaxConcurrent caps the number of layers downloaded in parallel.
	// 0 uses the backend default (Apple container CLI: 3). Only honored by
	// backends that expose this at the CLI — nerdctl does not.
	MaxConcurrent int
	// Progress selects the progress renderer: "ansi", "none", or "" for
	// auto-detect (ansi when stdout is a TTY, none otherwise). Prevents
	// ANSI redraw escape codes from cluttering piped/CI output.
	Progress string
}

// Compile-time check that Engine implements Runtime.
var _ Runtime = (*Engine)(nil)
