package cmd

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

// TestCLITree_LeafCommandsReject_UnusedArgs walks the entire command tree and
// invokes every leaf command with an unexpected positional arg. A leaf is
// compliant if either:
//   - it declares ArgsUsage (it intentionally accepts positional args), OR
//   - invoking it with a bogus arg returns an error
//
// Silent success on unused args is a bug — we had a real regression where
// `gocker images rm X` silently ran the list action instead of an error.
func TestCLITree_LeafCommandsReject_UnusedArgs(t *testing.T) {
	// Override urfave/cli's exit handler — by default cli.Exit errors trigger
	// os.Exit via cli.HandleExitCoder, which would terminate the test process.
	origExiter := cli.OsExiter
	cli.OsExiter = func(int) {}
	defer func() { cli.OsExiter = origExiter }()

	// Redirect cli.ErrWriter to discard — otherwise cli.Exit messages print
	// to the real os.Stderr (captured at urfave/cli's package init).
	origErrWriter := cli.ErrWriter
	cli.ErrWriter = io.Discard
	defer func() { cli.ErrWriter = origErrWriter }()

	// Suppress stdout/stderr from command Actions so test output stays clean.
	// Many leaf Actions print status strings (headers, "Daemon started", etc.)
	// even when invoked for no reason; we only care about the error return.
	restore := silenceStdio(t)
	defer restore()

	root := buildTestRoot(t)

	var nonCompliant []string
	walk(root, "", func(cmd *cli.Command, path string) {
		if len(cmd.Commands) > 0 {
			return // not a leaf
		}
		if cmd.ArgsUsage != "" {
			return // intentionally accepts args
		}
		if _, allowed := explicitlyAccepts[path]; allowed {
			return
		}
		if !actionRejectsExtras(cmd) {
			nonCompliant = append(nonCompliant, path)
		}
	})

	restore()
	if len(nonCompliant) > 0 {
		t.Errorf("leaf commands without ArgsUsage that silently accept extra args:\n  %s\n\n"+
			"Fix: either declare ArgsUsage on the command (if positional args are intended) "+
			"or reject extra args at the top of the Action, e.g.:\n\n"+
			"  if cmd.Args().Len() > 0 {\n"+
			"      return cli.Exit(\"unexpected arguments: \"+strings.Join(cmd.Args().Slice(), \" \"), 2)\n"+
			"  }",
			strings.Join(nonCompliant, "\n  "))
	}
}

// walk traverses the CLI tree, invoking fn for every command with the
// space-separated path from root.
func walk(cmd *cli.Command, prefix string, fn func(cmd *cli.Command, path string)) {
	path := cmd.Name
	if prefix != "" {
		path = prefix + " " + cmd.Name
	}
	fn(cmd, path)
	for _, sub := range cmd.Commands {
		walk(sub, path, fn)
	}
}

// actionRejectsExtras returns true if invoking cmd with a synthetic extra
// positional arg produces an error. Commands that would return an error
// because the runtime is a mock (not because they reject the arg) are
// indistinguishable from compliant here — that's an accepted trade-off; the
// goal is to catch *silent* success.
//
// We guard against panics from the mock runtime (unset Func fields panic on
// use) — a panic also counts as "not silently succeeding".
func actionRejectsExtras(cmd *cli.Command) (rejects bool) {
	if cmd.Action == nil {
		return true // no action = nothing to silently succeed
	}
	defer func() {
		if r := recover(); r != nil {
			rejects = true
		}
	}()
	ctx := context.Background()
	err := cmd.Run(ctx, []string{cmd.Name, "__gocker_bogus_positional_arg__"})
	return err != nil
}

// silenceStdio redirects os.Stdout and os.Stderr to /dev/null for the
// duration of the test. Returns a cleanup func.
func silenceStdio(t *testing.T) func() {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	os.Stdout = devnull
	os.Stderr = devnull
	var closed bool
	return func() {
		if closed {
			return
		}
		closed = true
		os.Stdout = origOut
		os.Stderr = origErr
		_ = devnull.Close()
	}
}

// explicitlyAccepts lists leaf commands that don't declare ArgsUsage but
// cannot be safely invoked in a unit test (network calls, subprocesses, etc.).
// Keys are the full space-separated path. Add each with a one-line comment.
var explicitlyAccepts = map[string]struct{}{
	"gocker daemon start":     {}, // forks a daemon subprocess via os.StartProcess
	"gocker daemon stop":      {}, // signals real pid from ~/.gocker/daemon.pid
	"gocker daemon status":    {}, // reads real pid file; benign but side-effecting
	"gocker daemon vm status": {}, // queries real shared VM via config.Load
	"gocker daemon vm stop":   {}, // calls vmMgr.Stop on the real VM name
	"gocker daemon vm rm":     {}, // calls vmMgr.Remove on the real VM name
	"gocker daemon vm update": {}, // pulls image, recreates VM
	"gocker setup":            {}, // shells out to sw_vers, may install binaries
}

// buildTestRoot constructs the CLI root by wiring every subcommand with a
// permissive mock runtime — every Runtime method returns success with empty
// results. This is deliberate: we want silent-swallow bugs to surface as
// silent SUCCESS, not as a runtime error that happens to mask the real
// problem. If a leaf command is genuinely broken (ignores extra args and
// falls through to a no-op runtime call), the mock will let it succeed and
// the test will flag it.
//
// Subcommands are invoked via their own cmd.Run, so NewApp's Before hook
// (which validates the real runtime) does not fire.
func buildTestRoot(t *testing.T) *cli.Command {
	t.Helper()
	mock := newPermissiveMockRuntime()
	return &cli.Command{
		Name: "gocker",
		Commands: []*cli.Command{
			newAICmd(mock),
			newBuildCmd(mock),
			newComposeCmd(mock),
			newDaemonCmd(mock),
			newExecCmd(mock),
			newImageCmd(mock),
			newImagesCmd(mock),
			newInfoCmd(mock, mock, "test"),
			newInspectCmd(mock),
			newLogsCmd(mock),
			newNetworkCmd(mock),
			newPsCmd(mock),
			newPullCmd(mock),
			newPushCmd(mock),
			newRmCmd(mock),
			newRmiCmd(mock),
			newRunCmd(mock),
			newSandboxCmd(mock),
			newSetupCmd(mock),
			newStartCmd(mock),
			newStopCmd(mock),
			newSystemCmd(mock, mock, "test"),
			newVolumeCmd(mock),
		},
	}
}

// newPermissiveMockRuntime returns a MockRuntime whose every method succeeds
// with empty/default values. Silent arg-swallow bugs should reach the runtime
// and succeed; the test detects that success as a failure.
func newPermissiveMockRuntime() *engine.MockRuntime {
	return &engine.MockRuntime{
		ValidateFunc:          func() error { return nil },
		BinaryPathFunc:        func() string { return "/mock/binary" },
		ExecFunc:              func(ctx context.Context, args ...string) ([]byte, []byte, error) { return nil, nil, nil },
		ExecInteractiveFunc:   func(ctx context.Context, args ...string) error { return nil },
		ExecStreamFunc:        func(ctx context.Context, args ...string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil },
		ContainerRunFunc:      func(ctx context.Context, args []string, interactive bool) error { return nil },
		ContainerListFunc:     func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) { return nil, nil },
		ContainerStopFunc:     func(ctx context.Context, nameOrID string) error { return nil },
		ContainerStartFunc:    func(ctx context.Context, nameOrID string) error { return nil },
		ContainerRemoveFunc:   func(ctx context.Context, nameOrID string, force bool) error { return nil },
		ContainerExecFunc:     func(ctx context.Context, nameOrID string, args []string, interactive bool) error { return nil },
		ContainerLogsFunc:     func(ctx context.Context, nameOrID string, follow bool) error { return nil },
		ContainerInspectFunc:  func(ctx context.Context, nameOrID string) ([]byte, error) { return []byte("[]"), nil },
		ImagePullFunc:         func(ctx context.Context, image string, opts engine.ImagePullOpts) error { return nil },
		ImagePushFunc:         func(ctx context.Context, image string) error { return nil },
		ImageListFunc:         func(ctx context.Context) ([]engine.ImageInfo, error) { return nil, nil },
		ImageRemoveFunc:       func(ctx context.Context, image string) error { return nil },
		ImageBuildFunc:        func(ctx context.Context, args []string) error { return nil },
		NetworkCreateFunc:     func(ctx context.Context, name string, labels map[string]string) error { return nil },
		NetworkListFunc:       func(ctx context.Context) ([]engine.NetworkInfo, error) { return nil, nil },
		NetworkRemoveFunc:     func(ctx context.Context, name string) error { return nil },
		NetworkConnectFunc:    func(ctx context.Context, network, container string) error { return nil },
		NetworkDisconnectFunc: func(ctx context.Context, network, container string) error { return nil },
		NetworkInspectFunc:    func(ctx context.Context, name string) ([]byte, error) { return []byte("{}"), nil },
		VolumeCreateFunc:      func(ctx context.Context, name string) error { return nil },
		VolumeListFunc:        func(ctx context.Context) ([]engine.VolumeInfo, error) { return nil, nil },
		VolumeRemoveFunc:      func(ctx context.Context, name string) error { return nil },
		VolumeInspectFunc:     func(ctx context.Context, name string) ([]byte, error) { return []byte("{}"), nil },
	}
}
