package fsutil

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWithLock_RunsFnAndReturnsItsError(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "area.lock")
	ran := false
	if err := WithLock(lockPath, func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("fn was not executed")
	}
}

func TestWithLock_HoldsExclusiveLockDuringFn(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "area.lock")
	err := WithLock(lockPath, func() error {
		// A second flock attempt on the same path must fail while held.
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			t.Error("expected lock to be held exclusively during fn")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithLock_ReleasesAfterFn(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "area.lock")
	if err := WithLock(lockPath, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Lock must be re-acquirable afterwards.
	if err := WithLock(lockPath, func() error { return nil }); err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
}

func TestWithLock_CreatesParentDir(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "newdir", "area.lock")
	if err := WithLock(lockPath, func() error { return nil }); err != nil {
		t.Fatalf("expected parent dir creation, got %v", err)
	}
}
