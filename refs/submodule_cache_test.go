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

// TestSubmoduleCacheServedRegardlessOfAge proves the content-addressing contract:
// NO time validity. An old entry is served while its content stamp is unchanged;
// a change to the repo-cache dir's content (a re-fetch) is an immediate miss.
func TestSubmoduleCacheServedRegardlessOfAge(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "charly.yml")
	t.Setenv("CHARLY_DEPLOY_CONFIG", cfg)
	writeSubmoduleCache("/tmp/repo1", true)
	path, _ := submoduleCachePath()
	data, _ := os.ReadFile(path)
	var cf cache.File
	_ = json.Unmarshal(data, &cf)
	for k, e := range cf.Entries {
		e.Written = time.Now().AddDate(-1, 0, 0)
		cf.Entries[k] = e
	}
	out, _ := json.Marshal(cf)
	_ = os.WriteFile(path, out, 0o644)
	if _, ok := readSubmoduleCache("/tmp/repo1"); !ok {
		t.Fatal("readSubmoduleCache: a present key must be served regardless of age (no TTL)")
	}
}

// TestSubmoduleCacheChangedContentIsAMiss: a re-fetch that populates submodules
// changes the content stamp -> a new key -> an immediate miss (the invalidation
// that used to wait for the 1h TTL).
func TestSubmoduleCacheChangedContentIsAMiss(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "charly.yml")
	t.Setenv("CHARLY_DEPLOY_CONFIG", cfg)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	// an UNPOPULATED export: declared submodule with no content
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"),
		[]byte("[submodule \"spec\"]\n\tpath = spec\n\turl = https://example.invalid/spec.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSubmoduleCache(repo, false)
	if v, ok := readSubmoduleCache(repo); !ok || v {
		t.Fatalf("unpopulated export: got %v ok %v", v, ok)
	}
	// the re-fetch POPULATES the submodule -> the content stamp changes
	if err := os.MkdirAll(filepath.Join(repo, "spec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "spec", "go.mod"), []byte("module y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := readSubmoduleCache(repo); ok {
		t.Fatal("a populated re-fetch must MISS (new content stamp), never serve the stale verdict")
	}
}
