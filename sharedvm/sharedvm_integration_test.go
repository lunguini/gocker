//go:build integration && darwin

package sharedvm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
)

// requireDestructiveTests skips tests that remove/recreate the shared VM
// unless GOCKER_DESTRUCTIVE_TESTS=1 is set. Every test here calls m.Remove,
// which would destroy a developer's in-use shared VM (and everything inside
// it) if run unguarded during `make test-all`.
func requireDestructiveTests(t *testing.T) {
	t.Helper()
	if os.Getenv("GOCKER_DESTRUCTIVE_TESTS") != "1" {
		t.Skip("skipping destructive test; set GOCKER_DESTRUCTIVE_TESTS=1 to run (removes/recreates the shared VM)")
	}
}

func integrationManager(t *testing.T) *Manager {
	t.Helper()
	eng := engine.New("")
	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Fatalf("system not running: %v", err)
	}
	// Wait for the system to be fully ready — after a system restart,
	// the API server may report "running" before it can accept container
	// operations, causing XPC connection interrupted errors.
	ctx := context.Background()
	for range 5 {
		if _, _, err := eng.Exec(ctx, "list", "--format", "json"); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	return NewManager(eng, config.SharedVM{
		Image:  "docker.io/adyjay/gocker:base-latest",
		Memory: "2G",
	})
}

func skipIfNoVirtualization(t *testing.T, err error) {
	t.Helper()
	if err != nil && (strings.Contains(err.Error(), "Virtualization") || strings.Contains(err.Error(), "XPC connection error")) {
		t.Skipf("skipping: %v", err)
	}
}

func TestIntegration_SharedVM_CreateAndStatus(t *testing.T) {
	requireDestructiveTests(t)
	m := integrationManager(t)
	ctx := context.Background()

	// Remove any existing VM
	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
		skipIfNoVirtualization(t, err)
		t.Fatalf("EnsureRunning failed: %v", err)
	}

	status := m.Status(ctx)
	if status != "running" {
		t.Errorf("expected status 'running', got %q", status)
	}
}

func TestIntegration_SharedVM_StopAndRestart(t *testing.T) {
	requireDestructiveTests(t)
	m := integrationManager(t)
	ctx := context.Background()

	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
		skipIfNoVirtualization(t, err)
		t.Fatalf("EnsureRunning failed: %v", err)
	}

	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status := m.Status(ctx)
	if status != "stopped" {
		t.Errorf("expected status 'stopped' after stop, got %q", status)
	}

	// Give the container system time to fully process the stop before restarting.
	// Without this, the XPC connection may still be tearing down, causing
	// "Connection interrupted" errors on the next operation.
	time.Sleep(2 * time.Second)

	if err := m.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning after stop failed: %v", err)
	}

	status = m.Status(ctx)
	if status != "running" {
		t.Errorf("expected status 'running' after restart, got %q", status)
	}
}

func TestIntegration_SharedVM_RemoveAndRecreate(t *testing.T) {
	requireDestructiveTests(t)
	m := integrationManager(t)
	ctx := context.Background()

	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
		skipIfNoVirtualization(t, err)
		t.Fatalf("first EnsureRunning failed: %v", err)
	}

	if err := m.Remove(ctx); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	status := m.Status(ctx)
	if status == "running" {
		t.Errorf("expected VM to not be running after Remove, got status %q", status)
	}

	if err := m.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning after Remove failed: %v", err)
	}

	status = m.Status(ctx)
	if status != "running" {
		t.Errorf("expected status 'running' after recreate, got %q", status)
	}
}

func TestIntegration_GetContainerStatus_RealInspect(t *testing.T) {
	requireDestructiveTests(t)
	m := integrationManager(t)
	ctx := context.Background()

	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
		skipIfNoVirtualization(t, err)
		t.Fatalf("EnsureRunning failed: %v", err)
	}

	status := m.getContainerStatus(ctx)
	if status != "running" {
		t.Errorf("expected getContainerStatus to return 'running', got %q", status)
	}
}
