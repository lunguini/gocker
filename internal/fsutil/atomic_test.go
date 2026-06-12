package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteFileAtomic(path, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("expected overwrite, got %s", data)
	}
}

func TestWriteFileAtomic_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := WriteFileAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestWriteFileAtomic_ErrorsOnMissingDir(t *testing.T) {
	err := WriteFileAtomic(filepath.Join(t.TempDir(), "nodir", "x.json"), []byte("x"), 0o644)
	if err == nil {
		t.Fatal("expected error for missing parent dir")
	}
	if !strings.Contains(err.Error(), "nodir") {
		t.Errorf("error should mention the path, got: %v", err)
	}
}
