package refs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGitClientDownloadRefusesDeadCachePath is the content-validity gate for the downloads
// cache (RCA 2026-09-06 — the same class the materialized-tree cache fixed with component
// drift detection): a cached PATH is only valid while its export EXISTS. A wiped or evicted
// repo-cache dir must NOT be served for the whole TTL — the downloader repopulates it on the
// next call. Removing the dirUsable guard fails this test (the second Download would serve
// the dead path with zero re-downloads).
func TestGitClientDownloadRefusesDeadCachePath(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "charly.yml")
	client := NewGitClient(cacheFile)

	// Materialize a "downloaded" export + persist its path in the downloads cache.
	export := filepath.Join(dir, "export")
	if err := os.MkdirAll(export, 0o755); err != nil {
		t.Fatalf("create export: %v", err)
	}
	client.mu.Lock()
	client.downloads["github.com/opencharly/example@v2026.240.0001"] = gitCacheEntry{Value: export, Resolved: time.Now()}
	client.save()
	client.mu.Unlock()

	calls := 0
	downloader := func(repoPath, version string) (string, error) {
		calls++
		return export, nil
	}

	got, err := client.Download("github.com/opencharly/example", "v2026.240.0001", downloader)
	if err != nil || got != export || calls != 0 {
		t.Fatalf("a LIVE cache path must be served without re-download (got=%q err=%v calls=%d)", got, err, calls)
	}

	// Evict the export (the repo-cache wipe): the persisted path is now DEAD.
	if err := os.RemoveAll(export); err != nil {
		t.Fatalf("evict export: %v", err)
	}
	got2, err := client.Download("github.com/opencharly/example", "v2026.240.0001", downloader)
	if err != nil || got2 != export || calls != 1 {
		t.Fatalf("a DEAD cache path must re-download (got=%q err=%v calls=%d, want calls=1)", got2, err, calls)
	}
}
