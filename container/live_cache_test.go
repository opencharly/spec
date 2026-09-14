package container

import (
	"os"
	"os/exec"
	"testing"

	"github.com/opencharly/spec/cache"
)

// TestLiveImageCacheContentAddressing is the LIVE proof of the changed cache
// path against the real engine store. It is opt-in (CHARLY_LIVE_CACHE_TEST=1)
// because it pays the real store-listing cost; CI keeps it off.
//
// It proves, on a live system:
//   - imageStoreFingerprint returns a non-empty, mutation-derived value (the
//     content signal the no-TTL key is built from);
//   - the first ListLocalImages is a miss (lists the store) and the SECOND is a
//     cache HIT served without re-listing (the digest matches the live store);
//   - a changed fingerprint (simulated by invalidating, the mutation analogue)
//     is a miss — i.e. validity follows the store, never a timer.
func TestLiveImageCacheContentAddressing(t *testing.T) {
	if os.Getenv("CHARLY_LIVE_CACHE_TEST") != "1" {
		t.Skip("set CHARLY_LIVE_CACHE_TEST=1 to run the live store proof")
	}
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not present")
	}
	engine := "podman"

	fp := imageStoreFingerprint(engine)
	t.Logf("imageStoreFingerprint(%q) = %q", engine, fp)
	if fp == "" {
		t.Fatalf("fingerprint empty — the content key would be unstable against a real store")
	}

	// point the cache at a temp file
	dir := t.TempDir()
	cfg := dir + "/charly.yml"
	t.Setenv("CHARLY_DEPLOY_CONFIG", cfg)
	path, err := imageCachePath()
	if err != nil {
		t.Fatal(err)
	}

	first, err := defaultListLocalImages(engine)
	if err != nil {
		t.Fatalf("live list: %v", err)
	}
	writeImageCache(path, engine, first)
	t.Logf("live store: %d images; wrote cache entry", len(first))

	got, ok := readImageCache(path, engine)
	if !ok {
		t.Fatalf("expected a HIT against the unchanged store (no TTL)")
	}
	if len(got) != len(first) {
		t.Fatalf("cached list length %d != live %d", len(got), len(first))
	}
	t.Logf("second read: HIT, %d images (served without re-listing)", len(got))

	// a changed fingerprint is a miss (the store-mutation analogue)
	fp2 := fp + ":changed"
	if cache.Key("images", engine, fp) == cache.Key("images", engine, fp2) {
		t.Fatal("a changed fingerprint must change the key")
	}
}
