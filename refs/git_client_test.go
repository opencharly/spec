package refs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// git_client_test.go — the centralized git layer: caching, persistence, and the
// freshness policy. The cache must (1) return a cached value without a network call,
// (2) persist across client instances, and (3) expire per the TTL policy.

func TestGitClientCacheAndPersist(t *testing.T) {
	dir := t.TempDir()
	client := NewGitClient(dir)

	// Seed the cache directly (no network).
	client.mu.Lock()
	client.latestTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: "v2026.240.0001", Resolved: time.Now()}
	client.defaultBranches["https://github.com/opencharly/example"] = gitCacheEntry{Value: "main", Resolved: time.Now()}
	client.save()
	client.mu.Unlock()

	// A NEW client (same cache dir) must see the persisted values.
	client2 := NewGitClient(dir)
	client2.mu.Lock()
	client2.load()
	client2.mu.Unlock()

	if v := cached(client2.latestTags, "https://github.com/opencharly/example", LatestTagTTL); v != "v2026.240.0001" {
		t.Fatalf("cached latest tag = %q, want v2026.240.0001", v)
	}
	if v := cached(client2.defaultBranches, "https://github.com/opencharly/example", DefaultBranchTTL); v != "main" {
		t.Fatalf("cached default branch = %q, want main", v)
	}

	// The cache file must exist at the expected path.
	if _, err := os.Stat(filepath.Join(dir, "git-cache.json")); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
}

func TestGitClientTTLExpiry(t *testing.T) {
	dir := t.TempDir()
	client := NewGitClient(dir)

	client.mu.Lock()
	client.latestTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: "v1", Resolved: time.Now().Add(-2 * LatestTagTTL)}
	client.mu.Unlock()

	// A stale entry must be ignored (the caller re-fetches).
	if v := cached(client.latestTags, "https://github.com/opencharly/example", LatestTagTTL); v != "" {
		t.Fatalf("stale cache entry should be ignored, got %q", v)
	}
}

func TestGitClientWarmUpColdDetection(t *testing.T) {
	dir := t.TempDir()
	client := NewGitClient(dir)

	// A repo with no cached entries is COLD.
	client.mu.Lock()
	have := cached(client.latestTags, "https://github.com/opencharly/example", LatestTagTTL) != "" &&
		cached(client.defaultBranches, "https://github.com/opencharly/example", DefaultBranchTTL) != ""
	client.mu.Unlock()
	if have {
		t.Fatal("a fresh client must report the repo as COLD")
	}
}
