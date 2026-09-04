package refs

import (
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

	// A NEW client (same cache file) must see the persisted values.
	client2 := NewGitClient(cacheFile)
	client2.mu.Lock()
	client2.load()
	client2.mu.Unlock()

	if v := cached(client2.latestTags, "https://github.com/opencharly/example", LatestTagTTL); v != "v2026.240.0001" {
		t.Fatalf("cached latest tag = %q, want v2026.240.0001", v)
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
