package refs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLatestTagCacheRoundTrip(t *testing.T) {
	// Point the cache at a temp dir.
	old := os.Getenv("CHARLY_REPO_CACHE")
	dir := t.TempDir()
	os.Setenv("CHARLY_REPO_CACHE", dir)
	defer func() {
		if old == "" {
			os.Unsetenv("CHARLY_REPO_CACHE")
		} else {
			os.Setenv("CHARLY_REPO_CACHE", old)
		}
	}()

	repo := "https://github.com/opencharly/example"
	if got := cachedLatestTag(repo); got != "" {
		t.Fatalf("cache should start empty, got %q", got)
	}
	rememberLatestTag(repo, "v2026.240.0001")
	if got := cachedLatestTag(repo); got != "v2026.240.0001" {
		t.Fatalf("cachedLatestTag = %q, want v2026.240.0001", got)
	}

	// A stale entry (older than the TTL) must be ignored.
	c, err := loadLatestTagCache()
	if err != nil {
		t.Fatal(err)
	}
	e := c.Entries[repo]
	e.Resolved = time.Now().Add(-2 * latestTagTTL)
	c.Entries[repo] = e
	if err := saveLatestTagCache(c); err != nil {
		t.Fatal(err)
	}
	if got := cachedLatestTag(repo); got != "" {
		t.Fatalf("stale cache entry should be ignored, got %q", got)
	}

	// The cache file must exist at the expected path.
	path := filepath.Join(dir, "latest-tags.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
}
