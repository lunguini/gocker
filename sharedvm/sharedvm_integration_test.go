//go:build integration && darwin

package sharedvm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
)

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

	// Skip if virtualization hardware is not available (e.g. GitHub Actions runners)
	const probe = "gocker-virt-probe"
	err := eng.ContainerRun(ctx, []string{"-d", "--name", probe, "alpine:latest", "true"}, false)
	_ = eng.ContainerRemove(ctx, probe, true)
	if err != nil && strings.Contains(err.Error(), "Virtualization is not available") {
		t.Skip("skipping: Virtualization.framework not available on this hardware")
	}

	return NewManager(eng, config.SharedVM{
		Image:  "docker.io/adyjay/gocker:base-latest",
		Memory: "2G",
	})
}

func TestIntegration_SharedVM_CreateAndStatus(t *testing.T) {
	m := integrationManager(t)
	ctx := context.Background()

	// Remove any existing VM
	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning failed: %v", err)
	}

	status := m.Status(ctx)
	if status != "running" {
		t.Errorf("expected status 'running', got %q", status)
	}
}

func TestIntegration_SharedVM_StopAndRestart(t *testing.T) {
	m := integrationManager(t)
	ctx := context.Background()

	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
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
	m := integrationManager(t)
	ctx := context.Background()

	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
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
	m := integrationManager(t)
	ctx := context.Background()

	_ = m.Remove(ctx)

	t.Cleanup(func() {
		_ = m.Remove(ctx)
	})

	if err := m.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning failed: %v", err)
	}

	status := m.getContainerStatus(ctx)
	if status != "running" {
		t.Errorf("expected getContainerStatus to return 'running', got %q", status)
	}
}
