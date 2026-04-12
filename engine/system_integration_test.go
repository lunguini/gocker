//go:build integration && darwin

package engine

import (
	"context"
	"strings"
	"testing"
)

func ensureSystemRunning(t *testing.T, eng *Engine) {
	t.Helper()
	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Fatalf("failed to ensure system running: %v", err)
	}
}

func TestIntegration_SystemStatus_WhenRunning(t *testing.T) {
	eng := New("")
	ensureSystemRunning(t, eng)

	stdout, _, err := eng.Exec(context.Background(), "system", "status")
	if err != nil {
		t.Fatalf("system status failed: %v", err)
	}
	if !strings.Contains(string(stdout), "running") {
		t.Errorf("expected stdout to contain 'running', got: %s", string(stdout))
	}
}

func TestIntegration_SystemStopAndRestart(t *testing.T) {
	eng := New("")
	ensureSystemRunning(t, eng)

	// Stop the system
	if err := eng.ExecInteractive(context.Background(), "system", "stop"); err != nil {
		t.Fatalf("system stop failed: %v", err)
	}

	// Check status after stop. launchd may auto-restart the service before
	// we can observe "not running", so just log the result rather than failing.
	stdout, stderr, _ := eng.Exec(context.Background(), "system", "status")
	combined := string(stdout) + string(stderr)
	if !strings.Contains(combined, "not running") {
		t.Logf("service auto-restarted before status check (launchd); got: %s", strings.TrimSpace(combined))
	}

	// Restart via EnsureSystemRunning
	ensureSystemRunning(t, eng)

	// Verify it's running again
	stdout, _, err := eng.Exec(context.Background(), "system", "status")
	if err != nil {
		t.Fatalf("system status failed after restart: %v", err)
	}
	if !strings.Contains(string(stdout), "running") {
		t.Errorf("expected stdout to contain 'running' after restart, got: %s", string(stdout))
	}
}

func TestIntegration_EnsureSystemRunning_Idempotent(t *testing.T) {
	eng := New("")

	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Fatalf("first EnsureSystemRunning failed: %v", err)
	}

	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Fatalf("second EnsureSystemRunning failed: %v", err)
	}
}
