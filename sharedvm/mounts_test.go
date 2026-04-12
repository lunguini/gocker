package sharedvm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslatePath_Covered(t *testing.T) {
	mounts := map[string]string{"/Users/adrian": "/host/Users/adrian"}
	got, ok := TranslatePath("/Users/adrian/code/app", mounts)
	if !ok {
		t.Fatal("expected ok=true for covered path")
	}
	if got != "/host/Users/adrian/code/app" {
		t.Errorf("got %q, want /host/Users/adrian/code/app", got)
	}
}

func TestTranslatePath_ExactMatch(t *testing.T) {
	mounts := map[string]string{"/Users/adrian": "/host/Users/adrian"}
	got, ok := TranslatePath("/Users/adrian", mounts)
	if !ok {
		t.Fatal("expected ok=true for exact match")
	}
	if got != "/host/Users/adrian" {
		t.Errorf("got %q, want /host/Users/adrian", got)
	}
}

func TestTranslatePath_NotCovered(t *testing.T) {
	mounts := map[string]string{"/Users/adrian": "/host/Users/adrian"}
	got, ok := TranslatePath("/opt/data/file.txt", mounts)
	if ok {
		t.Fatal("expected ok=false for uncovered path")
	}
	if got != "/opt/data/file.txt" {
		t.Errorf("got %q, want original path returned", got)
	}
}

func TestTranslateVolumeSpec_Covered(t *testing.T) {
	mounts := map[string]string{"/Users/adrian": "/host/Users/adrian"}
	got, err := TranslateVolumeSpec("/Users/adrian/app:/app", mounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/host/Users/adrian/app:/app" {
		t.Errorf("got %q, want /host/Users/adrian/app:/app", got)
	}
}

func TestTranslateVolumeSpec_NotCovered(t *testing.T) {
	mounts := map[string]string{"/Users/adrian": "/host/Users/adrian"}
	_, err := TranslateVolumeSpec("/opt/data:/data", mounts)
	if err == nil {
		t.Fatal("expected error for uncovered path")
	}
}

func TestTranslateVolumeSpec_NamedVolume(t *testing.T) {
	mounts := map[string]string{"/Users/adrian": "/host/Users/adrian"}
	got, err := TranslateVolumeSpec("myvolume:/data", mounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "myvolume:/data" {
		t.Errorf("got %q, want myvolume:/data", got)
	}
}

func TestTranslateVolumeSpec_NoColon(t *testing.T) {
	mounts := map[string]string{}
	got, err := TranslateVolumeSpec("myvolume", mounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "myvolume" {
		t.Errorf("got %q, want myvolume", got)
	}
}

func TestResolveMountParent_Directory(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveMountParent(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q (directory should mount itself)", got, dir)
	}
}

func TestResolveMountParent_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "task.txt")
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveMountParent(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q (file should mount parent dir)", got, dir)
	}
}

func TestResolveMountParent_NonexistentMountsParent(t *testing.T) {
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "subdir", "task.txt")
	got, err := ResolveMountParent(nonexistent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestResolveMountParent_BlockedRoot(t *testing.T) {
	_, err := ResolveMountParent("/tmp")
	if err == nil {
		t.Fatal("expected error for blocked path /tmp")
	}
}

func TestResolveMountParent_BlockedVar(t *testing.T) {
	_, err := ResolveMountParent("/var")
	if err == nil {
		t.Fatal("expected error for blocked path /var")
	}
}

func TestResolveMountParent_BlockedEtc(t *testing.T) {
	_, err := ResolveMountParent("/etc")
	if err == nil {
		t.Fatal("expected error for blocked path /etc")
	}
}

func TestResolveMountParent_BlockedPrivate(t *testing.T) {
	_, err := ResolveMountParent("/private")
	if err == nil {
		t.Fatal("expected error for blocked path /private")
	}
}

func TestResolveMountParent_BlockedSlash(t *testing.T) {
	_, err := ResolveMountParent("/")
	if err == nil {
		t.Fatal("expected error for blocked path /")
	}
}

func TestResolveMountParent_SubdirOfBlocked(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveMountParent(dir)
	if err != nil {
		t.Fatalf("unexpected error for subdir: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}
