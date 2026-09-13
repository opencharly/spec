package lock

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests pin the QUEUEING contract of a blocking acquire: a contended waiter waits for the
// lock (not 2 minutes and out), says what it is waiting on while it waits, and — if it ever does
// give up — names the HOLDER and the next step. The measured failure this closes: two concurrent
// check beds building the same image, the second failing hard with
// `flock <path>: lock held by another process for > 2m0s` (run 2026.255.2310) while the correct
// behaviour is to queue (the peer finishes, the waiter then cache-hits).

// TestAcquireFileLock_TimeoutNamesHolder: when the bounded wait DOES expire, the error must name
// the process holding the lock (pid + command, resolved from the kernel — never from lock-file
// bytes) and say what to do. Before this, the message named nobody: "lock held by another process
// for > 2m0s" left the operator with a path and no way to find the culprit.
func TestAcquireFileLock_TimeoutNamesHolder(t *testing.T) {
	oldTimeout := lockTimeout
	lockTimeout = 200 * time.Millisecond
	defer func() { lockTimeout = oldTimeout }()

	lockPath := filepath.Join(t.TempDir(), "held.lock")
	release, err := AcquireFileLock(lockPath, false) // this process holds it
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer func() { _ = release() }()

	_, err = AcquireFileLock(lockPath, true)
	if err == nil {
		t.Fatal("blocking acquire on a contended lock: expected an error after lockTimeout, got nil")
	}
	msg := err.Error()
	t.Logf("timeout error: %v", err)
	for _, want := range []string{lockPath, "still held by", fmt.Sprintf("pid %d", os.Getpid()), "200ms"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("timeout error = %q; want it to contain %q — a bounded failure must name the holder and the bound", msg, want)
		}
	}
	if !strings.Contains(msg, "nothing to clean up") {
		t.Fatalf("timeout error = %q; want the actionable next step (the flock is kernel-released on exit, so there is nothing to clean up)", msg)
	}
}

// TestAcquireFileLock_ReportsWhileWaiting pins the REPORT half of "bounded but reported": a
// legitimate queue can now last minutes (a cold image build), so a silent wait is indistinguishable
// from a hang. The waiter must say what it is waiting for, and who holds it, while it waits.
func TestAcquireFileLock_ReportsWhileWaiting(t *testing.T) {
	oldTimeout, oldInterval := lockTimeout, lockWaitReportInterval
	lockTimeout = 400 * time.Millisecond
	lockWaitReportInterval = 50 * time.Millisecond
	defer func() { lockTimeout, lockWaitReportInterval = oldTimeout, oldInterval }()

	lockPath := filepath.Join(t.TempDir(), "reported.lock")
	release, err := AcquireFileLock(lockPath, false)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer func() { _ = release() }()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	_, waitErr := AcquireFileLock(lockPath, true)
	os.Stderr = oldStderr
	if waitErr == nil {
		t.Fatal("blocking acquire on a contended lock: expected a timeout error, got nil")
	}
	_ = w.Close()
	report, _ := io.ReadAll(r)
	s := string(report)
	for _, want := range []string{"waiting", lockPath, fmt.Sprintf("pid %d", os.Getpid())} {
		if !strings.Contains(s, want) {
			t.Fatalf("wait report = %q; want it to contain %q — a queued build must be visible, not a silent stall", s, want)
		}
	}
}

// TestFlockHolderPIDs_NamesThisProcess is the positive control for the kernel-side holder lookup:
// with this process holding the flock, the inode scan must resolve exactly this pid. Without it the
// naming above could pass vacuously (a fallback string would satisfy the containment checks in the
// fallback direction only, but a broken scan would silently degrade every report to "unidentifiable").
func TestFlockHolderPIDs_NamesThisProcess(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "scan.lock")
	release, err := AcquireFileLock(lockPath, false)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer func() { _ = release() }()

	pids := flockHolderPIDs(lockPath)
	if len(pids) == 0 {
		t.Skip("holder lookup unavailable here (no /proc fdinfo locks) — the report degrades to the fallback text by design")
	}
	want := os.Getpid()
	for _, pid := range pids {
		if pid == want {
			return
		}
	}
	t.Fatalf("flockHolderPIDs(%s) = %v; want it to contain this pid %d", lockPath, pids, want)
}

// TestHolderDescription_FallsBackWhenUnheld: an UNHELD lock file has no kernel lock to resolve, so
// the description must degrade to plain text rather than an empty string (an empty holder in the
// message is worse than none: it reads like the report is broken).
func TestHolderDescription_FallsBackWhenUnheld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unheld.lock")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create lock file: %v", err)
	}
	if got := holderDescription(path); got == "" {
		t.Fatal("holderDescription(an unheld lock) = \"\"; want non-empty fallback text")
	}
}
