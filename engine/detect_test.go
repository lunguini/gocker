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

func TestResolveBinaryInfo_ReportsSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only PATH semantics")
	}

	path, source := ResolveBinaryInfo("/custom/container")
	if path != "/custom/container" || source != "config" {
		t.Errorf("override: got (%q, %q), want (/custom/container, config)", path, source)
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "container")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	path, source = ResolveBinaryInfo("")
	if path != bin || source != "PATH" {
		t.Errorf("PATH: got (%q, %q), want (%q, PATH)", path, source, bin)
	}

	t.Setenv("PATH", t.TempDir())
	path, source = ResolveBinaryInfo("")
	if path != "/usr/local/bin/container" || source != "fallback" {
		t.Errorf("fallback: got (%q, %q), want (/usr/local/bin/container, fallback)", path, source)
	}
}
