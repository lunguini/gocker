package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_BinaryExists(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "container")
	if err := os.WriteFile(tmp, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	eng := New(tmp)
	if err := eng.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_BinaryMissing(t *testing.T) {
	eng := New("/nonexistent/path/container")
	err := eng.Validate()
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	expected := "apple container CLI not found at /nonexistent/path/container (also not on PATH); run 'gocker setup' to install it, or set runtimeBinary in ~/.gocker/config.yaml if it lives elsewhere"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

// makeFakeBinary writes a shell script to dir/container and returns its path.
// The script dispatches on "$1 $2" (the first two arguments) to simulate
// different container CLI behaviours for EnsureSystemRunning tests.
func makeFakeBinary(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "container")
	content := fmt.Sprintf("#!/bin/sh\n%s\n", script)
	if err := os.WriteFile(binary, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary
}

// TestEnsureSystemRunning_AlreadyRunning — status exits 0 → returns nil, no start attempted.
func TestEnsureSystemRunning_AlreadyRunning(t *testing.T) {
	binary := makeFakeBinary(t, `
case "$1 $2" in
  "system status") exit 0 ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`)
	eng := New(binary)
	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestEnsureSystemRunning_NotRunning_StartsSuccessfully — status prints "apiserver is not running",
// exits 1; start exits 0; second status exits 0 → returns nil.
func TestEnsureSystemRunning_NotRunning_StartsSuccessfully(t *testing.T) {
	markerFile := filepath.Join(t.TempDir(), "started")
	binary := makeFakeBinary(t, fmt.Sprintf(`
MARKER="%s"
case "$1 $2" in
  "system status")
    if [ -f "$MARKER" ]; then
      exit 0
    fi
    echo "apiserver is not running"
    exit 1
    ;;
  "system start")
    touch "$MARKER"
    exit 0
    ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`, markerFile))
	eng := New(binary)
	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestEnsureSystemRunning_NonXPCError_PassesThrough — status prints "permission denied" to stderr,
// exits 1 → returns nil (doesn't attempt start).
func TestEnsureSystemRunning_NonXPCError_PassesThrough(t *testing.T) {
	binary := makeFakeBinary(t, `
case "$1 $2" in
  "system status")
    echo "permission denied" >&2
    exit 1
    ;;
  "system start")
    echo "should not be called" >&2
    exit 1
    ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`)
	eng := New(binary)
	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Errorf("expected nil (non-XPC error passes through), got %v", err)
	}
}

// TestEnsureSystemRunning_StartFails — status fails with "not running"; start exits 1 → returns error containing "failed to start".
func TestEnsureSystemRunning_StartFails(t *testing.T) {
	binary := makeFakeBinary(t, `
case "$1 $2" in
  "system status")
    echo "apiserver is not running"
    exit 1
    ;;
  "system start")
    exit 1
    ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`)
	eng := New(binary)
	err := eng.EnsureSystemRunning(context.Background())
	if err == nil {
		t.Fatal("expected error when start fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to start") {
		t.Errorf("expected error to contain 'failed to start', got %q", err.Error())
	}
}
