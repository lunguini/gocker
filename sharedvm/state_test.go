package sharedvm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadVMState(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	created := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	want := &VMState{
		Name:    "gocker-shared",
		Status:  "running",
		Image:   "docker.io/adyjay/gocker:base-latest",
		Created: created,
		Mounts:  map[string]string{"/Users/adrian": "/Users/adrian"},
	}

	if err := SaveVMState(want); err != nil {
		t.Fatalf("SaveVMState: %v", err)
	}

	got, err := LoadVMState()
	if err != nil {
		t.Fatalf("LoadVMState: %v", err)
	}

	if got.Name != want.Name {
		t.Errorf("Name: got %q, want %q", got.Name, want.Name)
	}
	if got.Status != want.Status {
		t.Errorf("Status: got %q, want %q", got.Status, want.Status)
	}
	if got.Image != want.Image {
		t.Errorf("Image: got %q, want %q", got.Image, want.Image)
	}
	if !got.Created.Equal(want.Created) {
		t.Errorf("Created: got %v, want %v", got.Created, want.Created)
	}
	if len(got.Mounts) != len(want.Mounts) {
		t.Errorf("Mounts length: got %d, want %d", len(got.Mounts), len(want.Mounts))
	}
	for k, v := range want.Mounts {
		if got.Mounts[k] != v {
			t.Errorf("Mounts[%q]: got %q, want %q", k, got.Mounts[k], v)
		}
	}
}

func TestLoadVMState_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	got, err := LoadVMState()
	if got != nil {
		t.Errorf("expected nil VMState for missing file, got %+v", got)
	}
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestDeleteVMState(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	state := &VMState{
		Name:   "gocker-shared",
		Status: "stopped",
	}
	if err := SaveVMState(state); err != nil {
		t.Fatalf("SaveVMState: %v", err)
	}

	stateFile := filepath.Join(tmpDir, "state.json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file should exist before delete: %v", err)
	}

	if err := DeleteVMState(); err != nil {
		t.Fatalf("DeleteVMState: %v", err)
	}

	if _, err := os.Stat(stateFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state file should be gone after delete, stat err: %v", err)
	}
}

func TestLoadVMState_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origStateDir := stateDir
	stateDir = func() string { return tmpDir }
	defer func() { stateDir = origStateDir }()

	stateFile := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(stateFile, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := LoadVMState()
	if got != nil {
		t.Errorf("expected nil VMState for corrupt JSON, got %+v", got)
	}
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}
