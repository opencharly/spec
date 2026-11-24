package perhost

// perhost_lock_test.go — Lock and Apply are the two halves Update is made of, and they must share
// ONE lock. A caller whose critical section is wider than one document update (deploykit's typed
// overlay cycle, `charly deploy reset`'s save-vs-remove decision) takes Lock and then calls Apply;
// if Apply re-acquired the lock, flock would make that same-process second acquire wait for the
// first — a self-deadlock that reads as a 10 s "still held by <yourself>" failure, or, with an
// unbounded acquire, a hang. Both are pinned here.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Apply under the caller's own Lock must WRITE, and must not wait for the lock.
func TestApplyWritesUnderTheCallersLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	release, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer func() { _ = release() }()

	done := make(chan error, 1)
	go func() {
		done <- Apply(path, func(root *yaml.Node) (bool, error) {
			return Set(root, scalar("v1"), "ledger", "deploys"), nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Apply under a held lock: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Apply blocked while the caller already holds Lock — it must not re-acquire the lock")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "ledger:") {
		t.Fatalf("Apply wrote nothing:\n%s", got)
	}
}

// Lock is the SAME lock Update takes: an Update started while the lock is held cannot complete, and
// completes as soon as it is released. This is the contract every wider cycle depends on.
func TestUpdateWaitsForTheCallersLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	release, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- Update(path, func(root *yaml.Node) (bool, error) {
			return Set(root, scalar("v1"), "cache", "git", "bypass"), nil
		})
	}()
	select {
	case err := <-done:
		_ = release()
		t.Fatalf("Update completed while its lock was held by the caller (err=%v) — Lock and Update must share one lock", err)
	case <-time.After(300 * time.Millisecond):
	}
	if uerr := release(); uerr != nil {
		t.Fatalf("release: %v", uerr)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Update after the lock was released: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Update did not acquire the lock after it was released")
	}
}

// A nil mutate is refused by both halves, so a caller cannot silently no-op its own write.
func TestApplyRefusesANilMutate(t *testing.T) {
	if err := Apply(filepath.Join(t.TempDir(), "charly.yml"), nil); err == nil {
		t.Fatal("Apply(nil) must be refused")
	}
}
