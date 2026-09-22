package container

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/opencharly/spec/cache"
)

// cache_test.go — the persistent caches for the status hot path. Each test
// FAILS without its behavior: the cache write/read/TTL/invalidation paths.

func TestImageCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := cache.Open(dir)
	images := []LocalImageInfo{
		{ID: "sha256:abc", Names: []string{"ghcr.io/opencharly/test:1.0"}, Labels: map[string]string{"ai.opencharly.box": "test"}},
	}
	writeImageCache(store, "podman", images)
	got, ok := readImageCache(store, "podman")
	if !ok {
		t.Fatal("readImageCache: cache miss after write")
	}
	if len(got) != 1 || got[0].ID != "sha256:abc" || got[0].Labels["ai.opencharly.box"] != "test" {
		t.Fatalf("readImageCache: got %+v", got)
	}
	// A different engine is a cache miss.
	if _, ok := readImageCache(store, "docker"); ok {
		t.Fatal("readImageCache: docker engine should miss")
	}
}

func TestImageCacheTTLExpiry(t *testing.T) {
	dir := t.TempDir()
	store := cache.Open(dir)
	images := []LocalImageInfo{{ID: "sha256:abc"}}
	writeImageCache(store, "podman", images)
	// Backdate the entry beyond the TTL through the shared Store's PutEntry seam.
	e, ok := store.Get("podman")
	if !ok {
		t.Fatal("store.Get: entry missing after write")
	}
	e.Resolved = time.Now().Add(-2 * imageCacheTTL)
	store.PutEntry("podman", e)
	if _, ok := readImageCache(store, "podman"); ok {
		t.Fatal("readImageCache: stale entry should miss")
	}
}

func TestInvalidateImageCache(t *testing.T) {
	// Point the cache at a temp root via the env override.
	dir := t.TempDir()
	t.Setenv("CHARLY_CACHE_DIR", dir)
	store := imageCacheStore()
	writeImageCache(store, "podman", []LocalImageInfo{{ID: "sha256:abc"}})
	InvalidateImageCache()
	if store.Len() != 0 {
		t.Fatalf("InvalidateImageCache: store still holds %d entries", store.Len())
	}
}

func TestImageLabelsCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := cache.Open(dir)
	labels := map[string]string{"ai.opencharly.box": "test"}
	key := "podman|ghcr.io/opencharly/test:1.0"
	writeImageLabelsCache(store, key, labels)
	got, ok := readImageLabelsCache(store, key)
	if !ok {
		t.Fatal("readImageLabelsCache: cache miss after write")
	}
	if got["ai.opencharly.box"] != "test" {
		t.Fatalf("readImageLabelsCache: got %v", got)
	}
	// A different key is a cache miss.
	if _, ok := readImageLabelsCache(store, "podman|other"); ok {
		t.Fatal("readImageLabelsCache: different key should miss")
	}
}

// TestImageCacheStoreDirIsNamed pins the store layout: the image list lives
// under a named `images/` dir in the cache root, so one Store per concern.
func TestImageCacheStoreDirIsNamed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHARLY_CACHE_DIR", root)
	if got, want := imageCacheStore().Dir(), filepath.Join(root, "images"); got != want {
		t.Fatalf("imageCacheStore().Dir() = %q, want %q", got, want)
	}
}
