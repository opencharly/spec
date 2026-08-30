package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAcquireFileLock_FailsFastOnContendedLock is the regression guard for the
// fleet-del stall: a blocking acquire on a lock held by another process must
// not hang the caller forever — it fails fast after lockTimeout.
func TestAcquireFileLock_FailsFastOnContendedLock(t *testing.T) {
	old := lockTimeout
	lockTimeout = 200 * time.Millisecond
	defer func() { lockTimeout = old }()

	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")
	// Hold the lock on a separate fd — the contended shape.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	release, err := AcquireFileLock(lockPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()

	start := time.Now()
	_, err = AcquireFileLock(lockPath, true)
	if err == nil {
		t.Fatal("blocking acquire on a contended lock: expected an error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("blocking acquire on a contended lock: took %s (unbounded hang)", elapsed)
	}
}
