package refs

// git_client_mu_test.go — save() must be callable while the caller holds g.mu.
//
// LatestTag and DefaultBranch mutate their maps under g.mu and then call save(); the cache tests
// seed the maps the same way. A save() that takes g.mu therefore self-deadlocks (sync.Mutex is not
// reentrant), which is why the GitClient's mu comment states the rule "save() must stay lock-free".
//
// This guard exists because that rule was broken while migrating save() onto the shared per-host
// writer: the helpers it called took g.mu, and TestGitClientCacheAndPersist hung for the full
// package timeout (measured 2026-11-24). A test that only checks WHAT was written cannot see a
// deadlock; this one checks that save() RETURNS.

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveIsCallableWhileHoldingMu(t *testing.T) {
	g := NewGitClient(filepath.Join(t.TempDir(), "charly.yml"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		g.mu.Lock()
		defer g.mu.Unlock()
		g.latestTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: "v1", Resolved: time.Now()}
		g.save()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("save() did not return while g.mu was held — it must stay lock-free (see the mu comment)")
	}
}
