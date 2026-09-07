package refs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// git_client_test.go — the centralized git layer: caching, persistence, and the
// freshness policy. The cache must (1) return a cached value without a network call,
// (2) persist across client instances, and (3) expire per the TTL policy. The cache
// lives in the `cache:` section of the per-host charly.yml — NOT a separate JSON file.

func TestGitClientCacheAndPersist(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "charly.yml")
	client := NewGitClient(cacheFile)

	// Seed the cache directly (no network).
	client.mu.Lock()
	client.latestTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: "v2026.240.0001", Resolved: time.Now()}
	client.defaultBranches["https://github.com/opencharly/example"] = gitCacheEntry{Value: "main", Resolved: time.Now()}
	client.save()
	client.mu.Unlock()

	// A NEW client (same cache file) must see the persisted values — with the
	// offline-fallback cutover, the persisted LATEST TAG loads as the FALLBACK
	// (never fresh data: a fresh process re-probes), while the other maps stay
	// plain TTL caches.
	client2 := NewGitClient(cacheFile)
	client2.mu.Lock()
	client2.load()
	client2.mu.Unlock()

	if v := cached(client2.latestTags, "https://github.com/opencharly/example", LatestTagTTL); v != "" {
		t.Fatalf("persisted latest tag leaked into the in-process fresh cache = %q, want empty (the persisted entry is the offline fallback, never fresh data)", v)
	}
	if v := client2.persistedTags["https://github.com/opencharly/example"].Value; v != "v2026.240.0001" {
		t.Fatalf("persisted offline-fallback latest tag = %q, want v2026.240.0001", v)
	}
	if v := cached(client2.defaultBranches, "https://github.com/opencharly/example", DefaultBranchTTL); v != "main" {
		t.Fatalf("cached default branch = %q, want main", v)
	}

	// The cache must be persisted in the `cache:` section of the charly.yml.
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
	var doc struct {
		Cache *struct {
			Git *struct {
				LatestTags map[string]gitCacheEntry `yaml:"latest_tags"`
			} `yaml:"git"`
		} `yaml:"cache"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("cache file is not valid YAML: %v", err)
	}
	if doc.Cache == nil || doc.Cache.Git == nil {
		t.Fatal("cache file has no cache: git: section")
	}
	if _, ok := doc.Cache.Git.LatestTags["https://github.com/opencharly/example"]; !ok {
		t.Fatal("cache: git: latest_tags missing the seeded entry")
	}
}

func TestGitClientPreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "charly.yml")

	// Pre-existing per-host config with deploy + provides keys.
	existing := "version: 2026.240.1943\nprovides:\n    mcp:\n        - name: jupyter\n          url: http://x:8888/mcp\nweb-local:\n    pod:\n        image: web\n"
	if err := os.WriteFile(cacheFile, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewGitClient(cacheFile)
	client.mu.Lock()
	client.latestTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: "v1", Resolved: time.Now()}
	client.save()
	client.mu.Unlock()

	// The other keys must survive the cache write.
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version  string `yaml:"version"`
		Provides *struct {
			MCP []struct {
				Name string `yaml:"name"`
			} `yaml:"mcp"`
		} `yaml:"provides"`
		WebLocal *struct {
			Pod *struct {
				Image string `yaml:"image"`
			} `yaml:"pod"`
		} `yaml:"web-local"`
		Cache *struct {
			Git *struct {
				LatestTags map[string]gitCacheEntry `yaml:"latest_tags"`
			} `yaml:"git"`
		} `yaml:"cache"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("cache file is not valid YAML: %v", err)
	}
	if doc.Version != "2026.240.1943" {
		t.Fatalf("version key lost: %q", doc.Version)
	}
	if doc.Provides == nil || len(doc.Provides.MCP) != 1 || doc.Provides.MCP[0].Name != "jupyter" {
		t.Fatal("provides key lost or corrupted")
	}
	if doc.WebLocal == nil || doc.WebLocal.Pod == nil || doc.WebLocal.Pod.Image != "web" {
		t.Fatal("deploy node lost or corrupted")
	}
	if doc.Cache == nil || doc.Cache.Git == nil {
		t.Fatal("cache: git: section missing")
	}
	if _, ok := doc.Cache.Git.LatestTags["https://github.com/opencharly/example"]; !ok {
		t.Fatal("cache: git: latest_tags missing the seeded entry")
	}
}

func TestGitClientFreshFileGetsVersionStamp(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "charly.yml")
	client := NewGitClient(cacheFile)

	// A fresh file (no pre-existing charly.yml) must be created WITH the HEAD
	// schema version stamp — the per-host file is loaded through the unified
	// loader, which rejects a version-less file.
	client.mu.Lock()
	client.latestTags["https://github.com/opencharly/example"] = gitCacheEntry{Value: "v1", Resolved: time.Now()}
	client.save()
	client.mu.Unlock()

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version string `yaml:"version"`
		Cache   *struct {
			Git *struct {
				LatestTags map[string]gitCacheEntry `yaml:"latest_tags"`
			} `yaml:"git"`
		} `yaml:"cache"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("cache file is not valid YAML: %v", err)
	}
	if doc.Version != spec.SchemaVersion {
		t.Fatalf("fresh cache file version = %q, want %q", doc.Version, spec.SchemaVersion)
	}
	if doc.Cache == nil || doc.Cache.Git == nil {
		t.Fatal("cache: git: section missing")
	}
}

func TestGitClientTTLExpiry(t *testing.T) {
	dir := t.TempDir()
	client := NewGitClient(filepath.Join(dir, "charly.yml"))

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
	client := NewGitClient(filepath.Join(dir, "charly.yml"))

	// A repo with no cached entries is COLD.
	client.mu.Lock()
	have := cached(client.latestTags, "https://github.com/opencharly/example", LatestTagTTL) != "" &&
		cached(client.defaultBranches, "https://github.com/opencharly/example", DefaultBranchTTL) != ""
	client.mu.Unlock()
	if have {
		t.Fatal("a fresh client must report the repo as COLD")
	}
}

// TestWarmUpSkipsPrefetchWhenPersistedFallbackExists pins the prefetch cold-check's
// persisted-awareness (the #108 warm-up-hang fix): a client whose PERSISTED latest_tags
// fallback already carries the repo must NOT re-probe the network in WarmUp — the
// pre-#108 model served the persisted entry as fresh, so the prefetch found the repo
// warm and returned instantly; #108 moved persisted entries to the offline-fallback
// map, WarmUp kept consulting only the in-process maps, and every fresh process
// re-probed the ENTIRE corpus (the observed 194-repo "first run" on every load). The
// prefetch is a UX batch, not a resolution: skipping it never serves stale data — the
// on-demand LatestTag keeps the strict re-probe contract (asserted by the fetch seam
// below staying un-called AND by the offline-fallback tests).
func TestWarmUpSkipsPrefetchWhenPersistedFallbackExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "charly.yml")
	repo := "https://github.com/opencharly/example"

	// Persist a latest_tags fallback entry the way a prior process's save() does:
	// through the real write path (LatestTag's save under the file lock), then read
	// it back in a FRESH client — the cross-process shape the bug lived in.
	{
		g := NewGitClient(file)
		g.mu.Lock()
		g.persistedTags[repo] = gitCacheEntry{Value: "v0.1.0", Resolved: time.Now().Add(-time.Hour)}
		// default_branches load into the live map and keep the TTL freshness
		// contract — seed FRESH (the 1h DefaultRefsCacheTTL), or the prefetch
		// correctly re-probes a stale default-branch answer.
		g.defaultBranches[repo] = gitCacheEntry{Value: "main", Resolved: time.Now()}
		g.save()
		g.mu.Unlock()
	}

	g := NewGitClient(file)
	g.mu.Lock()
	fallback := g.persistedTags[repo].Value
	g.mu.Unlock()
	if fallback != "v0.1.0" {
		t.Fatalf("fresh client loaded persisted tag %q, want v0.1.0", fallback)
	}

	// The fetch seam must stay UNCALLED: WarmUp skips the repo entirely.
	fetches := 0
	origFetch := gitLatestTagFetch
	gitLatestTagFetch = func(url string) (string, error) {
		fetches++
		return "", fmt.Errorf("unexpected network fetch for %s", url)
	}
	t.Cleanup(func() { gitLatestTagFetch = origFetch })

	g.WarmUp([]string{repo}, nil)
	if fetches != 0 {
		t.Fatalf("WarmUp fetched %d time(s) despite a persisted latest_tags fallback", fetches)
	}

	// A repo with NO persisted entry stays cold: the prefetch still fires for it
	// (the true first run), driving the (injected) fetch seam exactly once.
	g.WarmUp([]string{"https://github.com/opencharly/never-seen"}, nil)
	if fetches != 1 {
		t.Fatalf("WarmUp fetch count = %d, want 1 for the genuinely cold repo", fetches)
	}
}

func TestGitClientCacheSurface(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cache.yml")
	g := NewGitClient(file)

	// CacheStatus starts at zero entries.
	if _, n := g.CacheStatus(); n != 0 {
		t.Fatalf("fresh client: status %d entries, want 0", n)
	}

	// Prime the maps directly (no network): the cache is consulted before any fetch.
	g.mu.Lock()
	g.latestTags["repo/A"] = gitCacheEntry{Value: "v1", Resolved: time.Now()}
	g.resolvedRefs["repo/B main"] = gitCacheEntry{Value: "sha1", Resolved: time.Now()}
	g.mu.Unlock()

	if _, n := g.CacheStatus(); n != 2 {
		t.Fatalf("primed: status %d entries, want 2", n)
	}

	// BypassCache: the switch is the mechanism the three lookups honor before any
	// cached read (LatestTag/DefaultBranch/ResolveRef check !g.disabled first) —
	// observable via the field itself.
	g.BypassCache()
	if !g.disabled {
		t.Fatalf("bypass: disabled flag not set")
	}

	// ClearCache: entries drop + the persisted file is removed.
	if err := g.ClearCache(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, n := g.CacheStatus(); n != 0 {
		t.Fatalf("after clear: %d entries, want 0", n)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("after clear: cache file still present (%v)", err)
	}
}

func TestGitClientSetBypassPersists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cache.yml")

	// SetBypass(true) persists the flag: a FRESH client (a new process) honors it.
	g := NewGitClient(file)
	if err := g.SetBypass(true); err != nil {
		t.Fatalf("set bypass: %v", err)
	}
	if !g.disabled {
		t.Fatalf("set bypass: disabled flag not set")
	}

	fresh := NewGitClient(file)
	if !fresh.disabled {
		t.Fatalf("fresh client after SetBypass(true): bypass not honored at construction")
	}

	// SetBypass(false) clears it: a fresh client resumes the cache.
	if err := g.SetBypass(false); err != nil {
		t.Fatalf("clear bypass: %v", err)
	}
	fresh2 := NewGitClient(file)
	if fresh2.disabled {
		t.Fatalf("fresh client after SetBypass(false): bypass still set")
	}
}
