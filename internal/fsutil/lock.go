package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// WithLock runs fn while holding an exclusive advisory (flock) lock on
// lockPath, serializing concurrent state mutations across processes. The
// lock file's parent directory is created if missing. The lock is released
// when fn returns, even if fn panics.
func WithLock(lockPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer func() { _ = f.Close() }()

	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }()

	return fn()
}
