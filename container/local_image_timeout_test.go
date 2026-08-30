package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDefaultListLocalImages_FailsFastOnHungEngine is the regression guard for the
// silent-stall report: `podman images --format json` walks the whole local store, so on a
// large or contended store it has been observed to sit for 25+ minutes. Unbounded, that is
// indistinguishable from a hang — no log line, an empty run directory, and only a `podman
// images` child process as evidence. It must fail fast with an error that NAMES the cause.
func TestDefaultListLocalImages_FailsFastOnHungEngine(t *testing.T) {
	old := listLocalImagesTimeout
	listLocalImagesTimeout = 200 * time.Millisecond
	defer func() { listLocalImagesTimeout = old }()

	// A fake engine binary that never answers, the shape of a saturated store.
	dir := t.TempDir()
	engine := filepath.Join(dir, "podman")
	if err := os.WriteFile(engine, []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	done := make(chan error, 1)
	go func() {
		_, err := defaultListLocalImages("podman")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a hung engine must produce an error, not a nil result")
		}
		// The message has to say what to do about it — that is the point of the change.
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error does not name the timeout: %v", err)
		}
		if !strings.Contains(err.Error(), "prune") {
			t.Errorf("error does not tell the operator how to recover: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("defaultListLocalImages did not return within 10s — the call is still unbounded")
	}
}
