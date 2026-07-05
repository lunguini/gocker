package cmd

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/lunguini/gocker/engine"
)

// runtimeSwitch implements engine.Runtime by delegating every call to an
// atomically swappable inner Runtime. It exists so the CLI can honor a
// per-invocation --isolation flag: NewApp constructs one runtimeSwitch per
// role (general, sandbox) up front and hands it to every command
// constructor; the root Before hook then swaps in a re-resolved runtime
// once flags are parsed, if --isolation differs from the config-resolved
// default. Command constructors close over the switch itself (an
// engine.Runtime value), never the concrete runtime behind it, so the swap
// is invisible to them.
type runtimeSwitch struct {
	inner atomic.Pointer[engine.Runtime]
}

// newRuntimeSwitch returns a runtimeSwitch seeded with rt.
func newRuntimeSwitch(rt engine.Runtime) *runtimeSwitch {
	s := &runtimeSwitch{}
	s.Store(rt)
	return s
}

// Store atomically replaces the inner runtime.
func (s *runtimeSwitch) Store(rt engine.Runtime) {
	s.inner.Store(&rt)
}

// Load returns the current inner runtime.
func (s *runtimeSwitch) Load() engine.Runtime {
	return *s.inner.Load()
}

// Compile-time check that runtimeSwitch implements Runtime.
var _ engine.Runtime = (*runtimeSwitch)(nil)

func (s *runtimeSwitch) Validate() error    { return s.Load().Validate() }
func (s *runtimeSwitch) BinaryPath() string { return s.Load().BinaryPath() }

func (s *runtimeSwitch) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	return s.Load().Exec(ctx, args...)
}

func (s *runtimeSwitch) ExecInteractive(ctx context.Context, args ...string) error {
	return s.Load().ExecInteractive(ctx, args...)
}

func (s *runtimeSwitch) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	return s.Load().ExecStream(ctx, args...)
}

func (s *runtimeSwitch) ExecStreamSplit(ctx context.Context, args ...string) (stdout io.ReadCloser, stderr io.ReadCloser, err error) {
	return s.Load().ExecStreamSplit(ctx, args...)
}

func (s *runtimeSwitch) ExecStreamStdin(ctx context.Context, stdin io.Reader, args ...string) (stdout io.ReadCloser, stderr io.ReadCloser, err error) {
	return s.Load().ExecStreamStdin(ctx, stdin, args...)
}

func (s *runtimeSwitch) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	return s.Load().ContainerRun(ctx, args, interactive)
}

func (s *runtimeSwitch) ContainerCreate(ctx context.Context, args []string) (id string, err error) {
	return s.Load().ContainerCreate(ctx, args)
}

func (s *runtimeSwitch) ContainerList(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
	return s.Load().ContainerList(ctx, all)
}

func (s *runtimeSwitch) ContainerStop(ctx context.Context, nameOrID string) error {
	return s.Load().ContainerStop(ctx, nameOrID)
}

func (s *runtimeSwitch) ContainerStart(ctx context.Context, nameOrID string) error {
	return s.Load().ContainerStart(ctx, nameOrID)
}

func (s *runtimeSwitch) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	return s.Load().ContainerRemove(ctx, nameOrID, force)
}

func (s *runtimeSwitch) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	return s.Load().ContainerExec(ctx, nameOrID, args, interactive)
}

func (s *runtimeSwitch) ContainerLogs(ctx context.Context, nameOrID string, opts engine.LogsOptions) error {
	return s.Load().ContainerLogs(ctx, nameOrID, opts)
}

func (s *runtimeSwitch) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	return s.Load().ContainerInspect(ctx, nameOrID)
}

func (s *runtimeSwitch) ImagePull(ctx context.Context, image string, opts engine.ImagePullOpts) error {
	return s.Load().ImagePull(ctx, image, opts)
}

func (s *runtimeSwitch) ImagePush(ctx context.Context, image string) error {
	return s.Load().ImagePush(ctx, image)
}

func (s *runtimeSwitch) ImageList(ctx context.Context) ([]engine.ImageInfo, error) {
	return s.Load().ImageList(ctx)
}

func (s *runtimeSwitch) ImageRemove(ctx context.Context, image string) error {
	return s.Load().ImageRemove(ctx, image)
}

func (s *runtimeSwitch) ImageBuild(ctx context.Context, args []string) error {
	return s.Load().ImageBuild(ctx, args)
}

func (s *runtimeSwitch) NetworkCreate(ctx context.Context, name string, labels map[string]string) error {
	return s.Load().NetworkCreate(ctx, name, labels)
}

func (s *runtimeSwitch) NetworkList(ctx context.Context) ([]engine.NetworkInfo, error) {
	return s.Load().NetworkList(ctx)
}

func (s *runtimeSwitch) NetworkRemove(ctx context.Context, name string) error {
	return s.Load().NetworkRemove(ctx, name)
}

func (s *runtimeSwitch) NetworkConnect(ctx context.Context, network, container string) error {
	return s.Load().NetworkConnect(ctx, network, container)
}

func (s *runtimeSwitch) NetworkDisconnect(ctx context.Context, network, container string) error {
	return s.Load().NetworkDisconnect(ctx, network, container)
}

func (s *runtimeSwitch) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	return s.Load().NetworkInspect(ctx, name)
}

func (s *runtimeSwitch) VolumeCreate(ctx context.Context, name string) error {
	return s.Load().VolumeCreate(ctx, name)
}

func (s *runtimeSwitch) VolumeList(ctx context.Context) ([]engine.VolumeInfo, error) {
	return s.Load().VolumeList(ctx)
}

func (s *runtimeSwitch) VolumeRemove(ctx context.Context, name string) error {
	return s.Load().VolumeRemove(ctx, name)
}

func (s *runtimeSwitch) VolumeInspect(ctx context.Context, name string) ([]byte, error) {
	return s.Load().VolumeInspect(ctx, name)
}
