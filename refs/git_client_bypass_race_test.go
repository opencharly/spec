package refs

// git_client_bypass_race_test.go — the persisted bypass flag survives concurrent writers.
//
// save() writes the `cache: git: bypass` value. Before this guard it always wrote its OWN
// in-memory copy, so a resolution that had loaded `bypass: true` resurrected it on its next
// save, silently undoing `charly cache bypass --off`. Measured on a real host, 2026-11-24: the
// CLI printed "git-ref cache bypass cleared — the cache resumes" and the file read
// `bypass: true` again seconds later, from a concurrent client.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConcurrentSaveDoesNotResurrectAClearedBypass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")

	a := NewGitClient(path)
	if err := a.SetBypass(true); err != nil {
		t.Fatalf("SetBypass(true): %v", err)
	}

	// B starts while the bypass is ON, so it loads `true` and does not own the flag.
	b := NewGitClient(path)

	// The operator clears it through A — the CLI surface.
	if err := a.SetBypass(false); err != nil {
		t.Fatalf("SetBypass(false): %v", err)
	}

	// B then records an ordinary cache answer, which saves the file.
	b.mu.Lock()
	b.defaultBranches["https://github.com/opencharly/example"] = gitCacheEntry{Value: "main", Resolved: time.Now()}
	b.mu.Unlock()
	b.save()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "bypass: false") {
		t.Fatalf("a concurrent save resurrected the cleared bypass:\n%s", got)
	}
	if !strings.Contains(string(got), "opencharly/example") {
		t.Fatalf("the concurrent save must still record its own cache answer:\n%s", got)
	}
}

// And the OWNER still writes what it set: the fix must not freeze the flag forever.
func TestOwnerSaveStillWritesItsBypass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	g := NewGitClient(path)
	if err := g.SetBypass(true); err != nil {
		t.Fatalf("SetBypass(true): %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "bypass: true") {
		t.Fatalf("the process that changed the flag must persist it:\n%s", got)
	}
}
