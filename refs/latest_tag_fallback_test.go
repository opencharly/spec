package refs

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// latest_tag_fallback_test.go — the offline-fallback contract for the persisted
// latest_tags cache (the stale-latest-tag RCA fix): a fresh process with a
// reachable network ALWAYS re-probes (a tag pushed minutes ago must be seen), the
// persisted entry is served ONLY when the fetch fails, and the in-process 1h
// cache stays the batch-dedupe layer.

// fallbackFixtureClient builds a client whose persisted cache carries one
// STALE latest-tag entry (the anomaly shape: resolved well before a newer tag
// was pushed) in a temp per-host charly.yml, reconstructed from disk so the
// fixture proves the LOAD path (a fresh process).
func fallbackFixtureClient(t *testing.T, staleTag string, staleResolved time.Time) (*GitClient, string) {
	t.Helper()
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "charly.yml")
	client := NewGitClient(cacheFile)
	client.mu.Lock()
	client.persistedTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: staleTag, Resolved: staleResolved}
	client.save()
	client.mu.Unlock()
	return NewGitClient(cacheFile), cacheFile
}

// restoreFetchSeam swaps the LatestTag fetch seam and restores it on cleanup.
func restoreFetchSeam(t *testing.T, fetch func(string) (string, error), calls *atomic.Int32) {
	t.Helper()
	prev := gitLatestTagFetch
	gitLatestTagFetch = func(repoURL string) (string, error) {
		if calls != nil {
			calls.Add(1)
		}
		return fetch(repoURL)
	}
	t.Cleanup(func() { gitLatestTagFetch = prev })
}

// TestLatestTag_PersistedEntryIsOfflineFallbackOnly: persisted v2026.242.0932 + a
// reachable network serving v2026.249.2212 → the FRESH tag is returned and the
// persisted entry is updated. (The pre-fix behavior served the stale persisted
// tag for the whole 1h TTL — the reported plugin-resolution staleness anomaly.)
func TestLatestTag_PersistedEntryIsOfflineFallbackOnly(t *testing.T) {
	staleResolved := time.Now().Add(-40 * time.Minute) // well inside the old 1h TTL
	client, cacheFile := fallbackFixtureClient(t, "v2026.242.0932", staleResolved)

	restoreFetchSeam(t, func(string) (string, error) {
		return "v2026.249.2212", nil // the network sees the new tag
	}, nil)

	got, err := client.LatestTag("https://github.com/opencharly/example")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "v2026.249.2212" {
		t.Fatalf("LatestTag with reachable network = %q, want the FRESH v2026.249.2212 (the persisted entry is an offline fallback, never fresh data)", got)
	}

	// The fresh resolution replaces the stale fallback on disk.
	reloaded := NewGitClient(cacheFile)
	if v := reloaded.persistedTags["https://github.com/opencharly/example"].Value; v != "v2026.249.2212" {
		t.Fatalf("persisted entry after fresh fetch = %q, want v2026.249.2212", v)
	}
}

// TestLatestTag_OfflineFallbackOnFetchFailure: persisted entry + failed fetch →
// the persisted tag is served (resilience) — never an error while a fallback exists.
func TestLatestTag_OfflineFallbackOnFetchFailure(t *testing.T) {
	client, _ := fallbackFixtureClient(t, "v2026.242.0932", time.Now().Add(-40*time.Minute))

	restoreFetchSeam(t, func(string) (string, error) {
		return "", errors.New("dial tcp: connection refused (offline)")
	}, nil)

	got, err := client.LatestTag("https://github.com/opencharly/example")
	if err != nil {
		t.Fatalf("LatestTag offline with a persisted fallback: %v, want the fallback served", err)
	}
	if got != "v2026.242.0932" {
		t.Fatalf("offline LatestTag = %q, want the persisted fallback v2026.242.0932", got)
	}
}

// TestLatestTag_FetchFailureWithoutFallback: no persisted entry + failed fetch →
// the error propagates (no fake data is invented).
func TestLatestTag_FetchFailureWithoutFallback(t *testing.T) {
	client := NewGitClient(filepath.Join(t.TempDir(), "charly.yml"))
	restoreFetchSeam(t, func(string) (string, error) {
		return "", errors.New("dial tcp: connection refused (offline)")
	}, nil)
	if _, err := client.LatestTag("https://github.com/opencharly/example"); err == nil {
		t.Fatal("LatestTag offline without a fallback: nil error, want the fetch error propagated")
	}
}

// TestLatestTag_InProcessBatchDedupeStays: the second LatestTag call within the
// in-process TTL does NOT re-fetch — the batch-dedupe contract (the original
// 152-concurrent-ls-remote throttling fix) is untouched by the fallback cutover.
func TestLatestTag_InProcessBatchDedupeStays(t *testing.T) {
	client := NewGitClient(filepath.Join(t.TempDir(), "charly.yml"))
	var calls atomic.Int32
	restoreFetchSeam(t, func(string) (string, error) {
		return "v2026.249.2212", nil
	}, &calls)

	for range 2 {
		if _, err := client.LatestTag("https://github.com/opencharly/example"); err != nil {
			t.Fatalf("LatestTag: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("fetch seam called %d times for two in-process lookups, want 1 (the in-process batch dedupe must stay)", calls.Load())
	}
}
