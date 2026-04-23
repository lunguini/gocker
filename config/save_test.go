package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := Defaults()
	cfg.Isolation = "shared"
	cfg.SharedVM.CPUs = 6
	cfg.SharedVM.Memory = "8G"
	cfg.SharedVM.WorkspaceDirs = []string{"/Users/me"}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(tmp, ".gocker", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	loaded := Load()
	if loaded.Isolation != "shared" {
		t.Errorf("Isolation: got %q, want shared", loaded.Isolation)
	}
	if loaded.SharedVM.CPUs != 6 {
		t.Errorf("CPUs: got %d, want 6", loaded.SharedVM.CPUs)
	}
	if loaded.SharedVM.Memory != "8G" {
		t.Errorf("Memory: got %q, want 8G", loaded.SharedVM.Memory)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Save(Defaults()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".gocker")); err != nil {
		t.Errorf("expected ~/.gocker to be created: %v", err)
	}
}
