package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFrom_ValidConfig(t *testing.T) {
	path := writeConfigFile(t, "isolation: shared\n")
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Isolation != "shared" {
		t.Errorf("expected isolation shared, got %q", cfg.Isolation)
	}
}

func TestLoadFrom_MalformedYAMLReturnsError(t *testing.T) {
	path := writeConfigFile(t, "isolation: [unclosed\n")
	cfg, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected parse error for malformed YAML, got nil")
	}
	if cfg == nil {
		t.Fatal("expected defaults to be returned alongside the error")
	}
	if cfg.Isolation != "full" {
		t.Errorf("expected default isolation full, got %q", cfg.Isolation)
	}
}

func TestLoadFrom_MissingFileReturnsDefaultsNoError(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not be an error, got %v", err)
	}
	if cfg.Isolation != "full" {
		t.Errorf("expected default isolation full, got %q", cfg.Isolation)
	}
}

func TestLoadFrom_LegacyWorkspaceDirsMigrated(t *testing.T) {
	path := writeConfigFile(t, "workspaceDirs:\n  - /tmp/foo\n")
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.SharedVM.WorkspaceDirs) != 1 || cfg.SharedVM.WorkspaceDirs[0] != "/tmp/foo" {
		t.Errorf("legacy workspaceDirs not migrated: %#v", cfg.SharedVM.WorkspaceDirs)
	}
}
