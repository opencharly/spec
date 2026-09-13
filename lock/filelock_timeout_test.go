package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAcquireFileLock_BoundedOnContendedLock is the regression guard for the
// deploy-del stall: a blocking acquire on a lock held by another process must
// not hang the caller forever — it QUEUES and then fails after lockTimeout
// (renamed from ...FailsFast...: the bound still exists and still terminates,
// but "fast" stopped being true once a legitimate cold image build — the
// per-image lock's normal hold — measured ~26 minutes; see lockTimeout).
func TestAcquireFileLock_BoundedOnContendedLock(t *testing.T) {
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
