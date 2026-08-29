package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// cache_test.go — the persistent caches for the status hot path. Each test
// FAILS without its behavior: the cache write/read/TTL/invalidation paths.

func TestImageCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images.json")
	images := []LocalImageInfo{
		{ID: "sha256:abc", Names: []string{"ghcr.io/opencharly/test:1.0"}, Labels: map[string]string{"ai.opencharly.box": "test"}},
	}
	if err := writeImageCache(path, "podman", images); err != nil {
		t.Fatalf("writeImageCache: %v", err)
	}
	got, ok := readImageCache(path, "podman")
	if !ok {
		t.Fatal("readImageCache: cache miss after write")
	}
	if len(got) != 1 || got[0].ID != "sha256:abc" || got[0].Labels["ai.opencharly.box"] != "test" {
		t.Fatalf("readImageCache: got %+v", got)
	}
	// A different engine is a cache miss.
	if _, ok := readImageCache(path, "docker"); ok {
		t.Fatal("readImageCache: docker engine should miss")
	}
}

func TestImageCacheTTLExpiry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images.json")
	images := []LocalImageInfo{{ID: "sha256:abc"}}
	if err := writeImageCache(path, "podman", images); err != nil {
		t.Fatal(err)
	}
	// Backdate the resolution beyond the TTL.
	data, _ := os.ReadFile(path)
	var cf imageCacheFile
	json.Unmarshal(data, &cf)
	cf.Resolved = time.Now().Add(-2 * imageCacheTTL).UTC().Format(time.RFC3339)
	out, _ := json.Marshal(cf)
	_ = os.WriteFile(path, out, 0o644)
	if _, ok := readImageCache(path, "podman"); ok {
		t.Fatal("readImageCache: stale entry should miss")
	}
}

func TestInvalidateImageCache(t *testing.T) {
	// Point the cache at a temp file via the env override.
	dir := t.TempDir()
	cfg := filepath.Join(dir, "charly.yml")
	t.Setenv("CHARLY_DEPLOY_CONFIG", cfg)
	path, err := imageCachePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeImageCache(path, "podman", []LocalImageInfo{{ID: "sha256:abc"}}); err != nil {
		t.Fatal(err)
	}
	InvalidateImageCache()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("InvalidateImageCache: cache file still exists: %v", err)
	}
}

func TestImageLabelsCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "labels.json")
	labels := map[string]string{"ai.opencharly.box": "test"}
	if err := writeImageLabelsCache(path, "podman|ghcr.io/opencharly/test:1.0", labels); err != nil {
		t.Fatalf("writeImageLabelsCache: %v", err)
	}
	got, ok := readImageLabelsCache(path, "podman|ghcr.io/opencharly/test:1.0")
	if !ok {
		t.Fatal("readImageLabelsCache: cache miss after write")
	}
	if got["ai.opencharly.box"] != "test" {
		t.Fatalf("readImageLabelsCache: got %v", got)
	}
	// A different key is a cache miss.
	if _, ok := readImageLabelsCache(path, "podman|other"); ok {
		t.Fatal("readImageLabelsCache: different key should miss")
	}
}
