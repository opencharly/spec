package refs

import (
	"testing"
	"time"
)

// submodule_cache_test.go — the persistent submodule-populated verdict cache.
// Each test FAILS without its behavior.

func TestSubmoduleCacheRoundTrip(t *testing.T) {
	t.Setenv("CHARLY_CACHE_DIR", t.TempDir())
	writeSubmoduleCache("/tmp/repo1", true)
	writeSubmoduleCache("/tmp/repo2", false)
	got, ok := readSubmoduleCache("/tmp/repo1")
	if !ok || !got {
		t.Fatalf("readSubmoduleCache(repo1): got %v, ok %v", got, ok)
	}
	got, ok = readSubmoduleCache("/tmp/repo2")
	if !ok || got {
		t.Fatalf("readSubmoduleCache(repo2): got %v, ok %v", got, ok)
	}
	if _, ok := readSubmoduleCache("/tmp/repo3"); ok {
		t.Fatal("readSubmoduleCache(repo3): unknown path should miss")
	}
}

func TestSubmoduleCacheTTLExpiry(t *testing.T) {
	t.Setenv("CHARLY_CACHE_DIR", t.TempDir())
	writeSubmoduleCache("/tmp/repo1", true)
	// Backdate the entry beyond the TTL through the shared Store's PutEntry seam.
	store := submoduleCacheStore()
	e, ok := store.Get("/tmp/repo1")
	if !ok {
		t.Fatal("store.Get: entry missing after write")
	}
	e.Resolved = time.Now().Add(-2 * submoduleCacheTTL)
	_ = store.PutEntry("/tmp/repo1", e)
	if _, ok := readSubmoduleCache("/tmp/repo1"); ok {
		t.Fatal("readSubmoduleCache: stale entry should miss")
	}
}
