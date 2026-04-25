package engine

import (
	"context"
	"io"
)

// MockRuntime is a test double for the Runtime interface.
// Each method delegates to its corresponding function field.
// If a field is nil the method panics, except BinaryPath which returns "/mock/binary".
type MockRuntime struct {
	ValidateFunc           func() error
	BinaryPathFunc         func() string
	ExecFunc               func(ctx context.Context, args ...string) ([]byte, []byte, error)
	ExecInteractiveFunc    func(ctx context.Context, args ...string) error
	ExecStreamFunc         func(ctx context.Context, args ...string) (io.ReadCloser, error)
	ContainerRunFunc       func(ctx context.Context, args []string, interactive bool) error
	ContainerListFunc      func(ctx context.Context, all bool) ([]ContainerInfo, error)
	ContainerStopFunc      func(ctx context.Context, nameOrID string) error
	ContainerStartFunc     func(ctx context.Context, nameOrID string) error
	ContainerRemoveFunc    func(ctx context.Context, nameOrID string, force bool) error
	ContainerExecFunc      func(ctx context.Context, nameOrID string, args []string, interactive bool) error
	ContainerLogsFunc      func(ctx context.Context, nameOrID string, follow bool) error
	ContainerInspectFunc   func(ctx context.Context, nameOrID string) ([]byte, error)
	ImagePullFunc          func(ctx context.Context, image string, opts ImagePullOpts) error
	ImagePushFunc          func(ctx context.Context, image string) error
	ImageListFunc          func(ctx context.Context) ([]ImageInfo, error)
	ImageRemoveFunc        func(ctx context.Context, image string) error
	ImageBuildFunc         func(ctx context.Context, args []string) error
	NetworkCreateFunc      func(ctx context.Context, name string, labels map[string]string) error
	NetworkListFunc        func(ctx context.Context) ([]NetworkInfo, error)
	NetworkRemoveFunc      func(ctx context.Context, name string) error
	NetworkConnectFunc     func(ctx context.Context, network, container string) error
	NetworkDisconnectFunc  func(ctx context.Context, network, container string) error
	NetworkInspectFunc     func(ctx context.Context, name string) ([]byte, error)
	VolumeCreateFunc       func(ctx context.Context, name string) error
	VolumeListFunc         func(ctx context.Context) ([]VolumeInfo, error)
	VolumeRemoveFunc       func(ctx context.Context, name string) error
	VolumeInspectFunc      func(ctx context.Context, name string) ([]byte, error)
}

// Compile-time check that MockRuntime implements Runtime.
var _ Runtime = (*MockRuntime)(nil)

func (m *MockRuntime) Validate() error {
	if m.ValidateFunc == nil {
		panic("MockRuntime: Validate called but ValidateFunc is nil")
	}
	return m.ValidateFunc()
}

func (m *MockRuntime) BinaryPath() string {
	if m.BinaryPathFunc == nil {
		return "/mock/binary"
	}
	return m.BinaryPathFunc()
}

func (m *MockRuntime) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if m.ExecFunc == nil {
		panic("MockRuntime: Exec called but ExecFunc is nil")
	}
	return m.ExecFunc(ctx, args...)
}

func (m *MockRuntime) ExecInteractive(ctx context.Context, args ...string) error {
	if m.ExecInteractiveFunc == nil {
		panic("MockRuntime: ExecInteractive called but ExecInteractiveFunc is nil")
	}
	return m.ExecInteractiveFunc(ctx, args...)
}

func (m *MockRuntime) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	if m.ExecStreamFunc == nil {
		panic("MockRuntime: ExecStream called but ExecStreamFunc is nil")
	}
	return m.ExecStreamFunc(ctx, args...)
}

func (m *MockRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	if m.ContainerRunFunc == nil {
		panic("MockRuntime: ContainerRun called but ContainerRunFunc is nil")
	}
	return m.ContainerRunFunc(ctx, args, interactive)
}

func (m *MockRuntime) ContainerList(ctx context.Context, all bool) ([]ContainerInfo, error) {
	if m.ContainerListFunc == nil {
		panic("MockRuntime: ContainerList called but ContainerListFunc is nil")
	}
	return m.ContainerListFunc(ctx, all)
}

func (m *MockRuntime) ContainerStop(ctx context.Context, nameOrID string) error {
	if m.ContainerStopFunc == nil {
		panic("MockRuntime: ContainerStop called but ContainerStopFunc is nil")
	}
	return m.ContainerStopFunc(ctx, nameOrID)
}

func (m *MockRuntime) ContainerStart(ctx context.Context, nameOrID string) error {
	if m.ContainerStartFunc == nil {
		panic("MockRuntime: ContainerStart called but ContainerStartFunc is nil")
	}
	return m.ContainerStartFunc(ctx, nameOrID)
}

func (m *MockRuntime) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	if m.ContainerRemoveFunc == nil {
		panic("MockRuntime: ContainerRemove called but ContainerRemoveFunc is nil")
	}
	return m.ContainerRemoveFunc(ctx, nameOrID, force)
}

func (m *MockRuntime) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	if m.ContainerExecFunc == nil {
		panic("MockRuntime: ContainerExec called but ContainerExecFunc is nil")
	}
	return m.ContainerExecFunc(ctx, nameOrID, args, interactive)
}

func (m *MockRuntime) ContainerLogs(ctx context.Context, nameOrID string, follow bool) error {
	if m.ContainerLogsFunc == nil {
		panic("MockRuntime: ContainerLogs called but ContainerLogsFunc is nil")
	}
	return m.ContainerLogsFunc(ctx, nameOrID, follow)
}

func (m *MockRuntime) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	if m.ContainerInspectFunc == nil {
		panic("MockRuntime: ContainerInspect called but ContainerInspectFunc is nil")
	}
	return m.ContainerInspectFunc(ctx, nameOrID)
}

func (m *MockRuntime) ImagePull(ctx context.Context, image string, opts ImagePullOpts) error {
	if m.ImagePullFunc == nil {
		panic("MockRuntime: ImagePull called but ImagePullFunc is nil")
	}
	return m.ImagePullFunc(ctx, image, opts)
}

func (m *MockRuntime) ImagePush(ctx context.Context, image string) error {
	if m.ImagePushFunc == nil {
		panic("MockRuntime: ImagePush called but ImagePushFunc is nil")
	}
	return m.ImagePushFunc(ctx, image)
}

func (m *MockRuntime) ImageList(ctx context.Context) ([]ImageInfo, error) {
	if m.ImageListFunc == nil {
		panic("MockRuntime: ImageList called but ImageListFunc is nil")
	}
	return m.ImageListFunc(ctx)
}

func (m *MockRuntime) ImageRemove(ctx context.Context, image string) error {
	if m.ImageRemoveFunc == nil {
		panic("MockRuntime: ImageRemove called but ImageRemoveFunc is nil")
	}
	return m.ImageRemoveFunc(ctx, image)
}

func (m *MockRuntime) ImageBuild(ctx context.Context, args []string) error {
	if m.ImageBuildFunc == nil {
		panic("MockRuntime: ImageBuild called but ImageBuildFunc is nil")
	}
	return m.ImageBuildFunc(ctx, args)
}

func (m *MockRuntime) NetworkCreate(ctx context.Context, name string, labels map[string]string) error {
	if m.NetworkCreateFunc == nil {
		panic("MockRuntime: NetworkCreate called but NetworkCreateFunc is nil")
	}
	return m.NetworkCreateFunc(ctx, name, labels)
}

func (m *MockRuntime) NetworkList(ctx context.Context) ([]NetworkInfo, error) {
	if m.NetworkListFunc == nil {
		panic("MockRuntime: NetworkList called but NetworkListFunc is nil")
	}
	return m.NetworkListFunc(ctx)
}

func (m *MockRuntime) NetworkRemove(ctx context.Context, name string) error {
	if m.NetworkRemoveFunc == nil {
		panic("MockRuntime: NetworkRemove called but NetworkRemoveFunc is nil")
	}
	return m.NetworkRemoveFunc(ctx, name)
}

func (m *MockRuntime) NetworkConnect(ctx context.Context, network, container string) error {
	if m.NetworkConnectFunc == nil {
		panic("MockRuntime: NetworkConnect called but NetworkConnectFunc is nil")
	}
	return m.NetworkConnectFunc(ctx, network, container)
}

func (m *MockRuntime) NetworkDisconnect(ctx context.Context, network, container string) error {
	if m.NetworkDisconnectFunc == nil {
		panic("MockRuntime: NetworkDisconnect called but NetworkDisconnectFunc is nil")
	}
	return m.NetworkDisconnectFunc(ctx, network, container)
}

func (m *MockRuntime) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	if m.NetworkInspectFunc == nil {
		panic("MockRuntime: NetworkInspect called but NetworkInspectFunc is nil")
	}
	return m.NetworkInspectFunc(ctx, name)
}

func (m *MockRuntime) VolumeCreate(ctx context.Context, name string) error {
	if m.VolumeCreateFunc == nil {
		panic("MockRuntime: VolumeCreate called but VolumeCreateFunc is nil")
	}
	return m.VolumeCreateFunc(ctx, name)
}

func (m *MockRuntime) VolumeList(ctx context.Context) ([]VolumeInfo, error) {
	if m.VolumeListFunc == nil {
		panic("MockRuntime: VolumeList called but VolumeListFunc is nil")
	}
	return m.VolumeListFunc(ctx)
}

func (m *MockRuntime) VolumeRemove(ctx context.Context, name string) error {
	if m.VolumeRemoveFunc == nil {
		panic("MockRuntime: VolumeRemove called but VolumeRemoveFunc is nil")
	}
	return m.VolumeRemoveFunc(ctx, name)
}

func (m *MockRuntime) VolumeInspect(ctx context.Context, name string) ([]byte, error) {
	if m.VolumeInspectFunc == nil {
		panic("MockRuntime: VolumeInspect called but VolumeInspectFunc is nil")
	}
	return m.VolumeInspectFunc(ctx, name)
}
