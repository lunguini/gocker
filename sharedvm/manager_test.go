package sharedvm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
)

func newTestManager(rt engine.Runtime) *Manager {
	return &Manager{
		apple:  rt,
		config: config.SharedVM{},
		mounts: map[string]string{},
		name:   vmName,
	}
}

// ---- getContainerStatus tests ----

func TestGetContainerStatus_JSONArrayWithStatus(t *testing.T) {
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"status":"running","configuration":{"id":"gocker-shared"}}]`), nil
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "running" {
		t.Errorf("expected \"running\", got %q", got)
	}
}

func TestGetContainerStatus_JSONObjectWithStatus(t *testing.T) {
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`{"status":"stopped"}`), nil
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "stopped" {
		t.Errorf("expected \"stopped\", got %q", got)
	}
}

func TestGetContainerStatus_NestedStatusInArray(t *testing.T) {
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"configuration":{"id":"gocker-shared","resources":{"memoryInBytes":4294967296}},"networks":[],"startedDate":796604637.228884,"status":"running"}]`), nil
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "running" {
		t.Errorf("expected \"running\", got %q", got)
	}
}

func TestGetContainerStatus_InspectError(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return nil, errors.New("container not found")
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "" {
		t.Errorf("expected \"\", got %q", got)
	}
}

func TestGetContainerStatus_EmptyArray(t *testing.T) {
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[]`), nil
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "unknown" {
		t.Errorf("expected \"unknown\", got %q", got)
	}
}

func TestGetContainerStatus_NoStatusField(t *testing.T) {
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"configuration":{"id":"gocker-shared"}}]`), nil
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "unknown" {
		t.Errorf("expected \"unknown\", got %q", got)
	}
}

func TestGetContainerStatus_InvalidJSON(t *testing.T) {
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`not json at all`), nil
		},
	}
	m := newTestManager(rt)
	got := m.getContainerStatus(context.Background())
	if got != "unknown" {
		t.Errorf("expected \"unknown\", got %q", got)
	}
}

// ---- EnsureRunning state machine tests ----

func TestEnsureRunning_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"status":"running"}]`), nil
		},
	}
	m := newTestManager(rt)
	err := m.EnsureRunning(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestEnsureRunning_Stopped_Starts(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	var startedName string
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"status":"stopped"}]`), nil
		},
		ContainerStartFunc: func(ctx context.Context, nameOrID string) error {
			startedName = nameOrID
			return nil
		},
	}
	m := newTestManager(rt)
	err := m.EnsureRunning(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if startedName != vmName {
		t.Errorf("expected ContainerStart called with %q, got %q", vmName, startedName)
	}
}

func TestEnsureRunning_Missing_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	var removeCalled bool
	var runCalled bool
	var runInteractive bool
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return nil, errors.New("no such container")
		},
		ExecFunc: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			// Before ContainerRun: probe fails (VM doesn't exist).
			// After ContainerRun: readiness probe succeeds (VM is up).
			if runCalled {
				return nil, nil, nil
			}
			return nil, nil, errors.New("container not running")
		},
		ContainerRemoveFunc: func(ctx context.Context, nameOrID string, force bool) error {
			removeCalled = true
			return nil
		},
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			runCalled = true
			runInteractive = interactive
			return nil
		},
	}
	m := newTestManager(rt)
	err := m.EnsureRunning(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if !removeCalled {
		t.Error("expected ContainerRemove to be called for orphan cleanup")
	}
	if !runCalled {
		t.Error("expected ContainerRun to be called")
	}
	if runInteractive {
		t.Error("expected ContainerRun to be called with interactive=false")
	}
}

func TestEnsureRunning_CreateFails_CleansUp(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	removeCount := 0
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return nil, errors.New("no such container")
		},
		ExecFunc: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			return nil, nil, errors.New("container not running")
		},
		ContainerRemoveFunc: func(ctx context.Context, nameOrID string, force bool) error {
			removeCount++
			return nil
		},
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			return errors.New("run failed")
		},
	}
	m := newTestManager(rt)
	err := m.EnsureRunning(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "creating shared VM") {
		t.Errorf("expected error to contain \"creating shared VM\", got %q", err.Error())
	}
	// First remove is orphan cleanup, second is failure cleanup
	if removeCount < 2 {
		t.Errorf("expected ContainerRemove called at least 2 times, got %d", removeCount)
	}
}

func TestEnsureRunning_InspectFails_ProbeSucceeds_SkipsCreate(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	// Pre-populate state file saying VM is running
	_ = SaveVMState(&VMState{Name: vmName, Status: "running", Image: "test"})

	var createCalled bool
	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return nil, errors.New("transient error")
		},
		ExecFunc: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			// Probe succeeds — VM is actually alive
			return nil, nil, nil
		},
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			createCalled = true
			return nil
		},
	}
	m := newTestManager(rt)
	err := m.EnsureRunning(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if createCalled {
		t.Error("expected ContainerRun NOT to be called when probe succeeds")
	}
}

func TestEnsureRunning_StartFails(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	rt := &engine.MockRuntime{
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"status":"stopped"}]`), nil
		},
		ContainerStartFunc: func(ctx context.Context, nameOrID string) error {
			return errors.New("start failed")
		},
	}
	m := newTestManager(rt)
	err := m.EnsureRunning(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "starting shared VM") {
		t.Errorf("expected error to contain \"starting shared VM\", got %q", err.Error())
	}
}

func TestExpandMounts_AddsNewMount(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	var removeCalled, runCalled bool
	rt := &engine.MockRuntime{
		ContainerListFunc: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return nil, nil
		},
		ContainerInspectFunc: func(ctx context.Context, nameOrID string) ([]byte, error) {
			if runCalled {
				return []byte(`[{"status":"running"}]`), nil
			}
			return nil, errors.New("not found")
		},
		ContainerRemoveFunc: func(ctx context.Context, nameOrID string, force bool) error {
			removeCalled = true
			return nil
		},
		ContainerStopFunc: func(ctx context.Context, nameOrID string) error {
			return nil
		},
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			runCalled = true
			return nil
		},
		ExecFunc: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			if runCalled {
				return nil, nil, nil
			}
			return nil, nil, errors.New("not running")
		},
	}

	m := newTestManager(rt)
	m.mounts = map[string]string{"/Users/adrian": "/host/Users/adrian"}

	err := m.ExpandMounts(context.Background(), []string{"/opt/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !removeCalled {
		t.Error("expected VM to be removed for recreation")
	}
	if !runCalled {
		t.Error("expected VM to be recreated")
	}
	if _, ok := m.mounts["/opt/data"]; !ok {
		t.Error("expected /opt/data to be added to mounts")
	}
}

func TestExpandMounts_AlreadyCovered(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	rt := &engine.MockRuntime{}
	m := newTestManager(rt)
	m.mounts = map[string]string{"/Users/adrian": "/host/Users/adrian"}

	err := m.ExpandMounts(context.Background(), []string{"/Users/adrian/code"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// When the VM holds state (containers/images) and the caller is
// non-interactive with no override, recreation is refused rather than
// silently destroying that state.
func TestExpandMounts_VMHasState_NonInteractiveRefuses(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	var runCalled bool
	rt := &engine.MockRuntime{
		ExecFunc: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			// Any nerdctl ps/images probe reports one item — the VM has state.
			return []byte("abc123\n"), nil, nil
		},
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			runCalled = true
			return nil
		},
	}
	m := newTestManager(rt)
	m.mounts = map[string]string{"/Users/adrian": "/host/Users/adrian"}

	err := m.ExpandMounts(context.Background(), []string{"/opt/data"})
	if err == nil {
		t.Fatal("expected refusal when the VM holds state and no confirmation is available")
	}
	if !strings.Contains(err.Error(), "Refusing without confirmation") {
		t.Errorf("expected refusal error, got: %v", err)
	}
	if runCalled {
		t.Error("VM must not be recreated without confirmation")
	}
	if _, ok := m.mounts["/opt/data"]; ok {
		t.Error("mounts must not be mutated when recreation is refused")
	}
}

// With GOCKER_ASSUME_YES set, the same state does not block recreation.
func TestExpandMounts_VMHasState_AssumeYesProceeds(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()
	t.Setenv("GOCKER_ASSUME_YES", "1")

	var runCalled bool
	rt := &engine.MockRuntime{
		ContainerStopFunc: func(ctx context.Context, nameOrID string) error { return nil },
		ContainerRemoveFunc: func(ctx context.Context, nameOrID string, force bool) error {
			return nil
		},
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			runCalled = true
			return nil
		},
		ExecFunc: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			// State probes report an image; readiness probe after run succeeds.
			if runCalled {
				return nil, nil, nil
			}
			return []byte("img1\n"), nil, nil
		},
	}
	m := newTestManager(rt)
	m.mounts = map[string]string{"/Users/adrian": "/host/Users/adrian"}

	if err := m.ExpandMounts(context.Background(), []string{"/opt/data"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !runCalled {
		t.Error("expected VM to be recreated when GOCKER_ASSUME_YES is set")
	}
	if _, ok := m.mounts["/opt/data"]; !ok {
		t.Error("expected /opt/data added to mounts after successful recreate")
	}
}
