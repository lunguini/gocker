package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveContainerBinary_OverrideWins(t *testing.T) {
	got := resolveContainerBinary("/custom/path/container")
	if got != "/custom/path/container" {
		t.Errorf("override should win, got %q", got)
	}
}

func TestResolveContainerBinary_FindsBinaryOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only PATH semantics")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "container")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got := resolveContainerBinary("")
	if got != bin {
		t.Errorf("expected PATH-resolved %q, got %q", bin, got)
	}
}

func TestResolveContainerBinary_FallsBackToUsrLocalBin(t *testing.T) {
	// Empty PATH entry pointing at a dir with no `container` binary.
	t.Setenv("PATH", t.TempDir())

	got := resolveContainerBinary("")
	if got != "/usr/local/bin/container" {
		t.Errorf("expected fallback /usr/local/bin/container, got %q", got)
	}
}
