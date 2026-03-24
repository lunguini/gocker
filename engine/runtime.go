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
	ContainerLogs(ctx context.Context, nameOrID string, follow bool) error
	ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error)

	// Images
	ImagePull(ctx context.Context, image string) error
	ImagePush(ctx context.Context, image string) error
	ImageList(ctx context.Context) ([]ImageInfo, error)
	ImageRemove(ctx context.Context, image string) error
	ImageBuild(ctx context.Context, args []string) error

	// Networks
	NetworkCreate(ctx context.Context, name string) error
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

// Compile-time check that Engine implements Runtime.
var _ Runtime = (*Engine)(nil)
