package lock

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAcquireFileLockWithin_ShortBoundFailsFast pins the config-write bound: a
// brief critical section (a deploy-config read-modify-write) must NOT inherit the
// 30-minute image-build bound. A contended short-bound acquire fails fast with a
// clear, named message — the RCA-issue-2 fix (a cross-bed overlay collision used
// to present as a silent 30-minute hang, indistinguishable from a slow build).
func TestAcquireFileLockWithin_ShortBoundFailsFast(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "cfg-charly.yml.lock")
	release, err := AcquireFileLock(lockPath, false)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer func() { _ = release() }()

	start := time.Now()
	_, err = AcquireFileLockWithin(lockPath, true, 150*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("short-bound acquire on a contended lock: expected an error, got nil")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("short-bound acquire took %s — it inherited the long image-build bound", elapsed)
	}
	// The failure must be actionable: it names the path, the holder, and the bound.
	msg := err.Error()
	for _, want := range []string{lockPath, "still held by", "150ms"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("short-bound error = %q; want it to contain %q", msg, want)
		}
	}
}
