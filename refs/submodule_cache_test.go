package refs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencharly/spec/cache"
)

// submodule_cache_test.go — the persistent submodule-populated verdict cache.
// Each test FAILS without its behavior.

func TestSubmoduleCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "charly.yml")
	t.Setenv("CHARLY_DEPLOY_CONFIG", cfg)
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
	dir := t.TempDir()
	cfg := filepath.Join(dir, "charly.yml")
	t.Setenv("CHARLY_DEPLOY_CONFIG", cfg)
	writeSubmoduleCache("/tmp/repo1", true)
	// Backdate the entry beyond the TTL via the shared cache file.
	path, _ := submoduleCachePath()
	data, _ := os.ReadFile(path)
	var cf cache.File
	json.Unmarshal(data, &cf)
	for k, e := range cf.Entries {
		e.Resolved = time.Now().Add(-2 * submoduleCacheTTL)
		cf.Entries[k] = e
	}
	out, _ := json.Marshal(cf)
	_ = os.WriteFile(path, out, 0o644)
	if _, ok := readSubmoduleCache("/tmp/repo1"); ok {
		t.Fatal("readSubmoduleCache: stale entry should miss")
	}
}
