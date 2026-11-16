package cache

import (
	"testing"

	"github.com/opencharly/spec/storeroot"
)

// TestStoreRootIsTheOneResolver pins D-STORE-1 on the store-root path: the roots are resolved by ONE
// resolver on the contract surface (`spec/storeroot`), so the plugin host and every plugin it serves
// resolve the same tree from the same code.
//
// This FAILS without the change. `StoreRoot` used to re-derive the cache root itself — reading
// CHARLY_CACHE_DIR, then falling back to `filepath.Dir(spec.DefaultDeployConfigPath())/cache` — a
// SECOND copy of the rule. The two could disagree (they ordered the override and the config-derived
// fallback differently), which is how a host and a plugin end up writing to different cache trees
// while both believe they resolved "the cache".
func TestStoreRootIsTheOneResolver(t *testing.T) {
	t.Run("agrees with storeroot.CacheDir by default", func(t *testing.T) {
		got, err := StoreRoot()
		if err != nil {
			t.Fatalf("StoreRoot: %v", err)
		}
		want, err := storeroot.CacheDir()
		if err != nil {
			t.Fatalf("storeroot.CacheDir: %v", err)
		}
		if got != want {
			t.Fatalf("StoreRoot = %q but the ONE resolver says %q — two rules for one tree", got, want)
		}
	})

	t.Run("agrees with storeroot.CacheDir under the single override", func(t *testing.T) {
		t.Setenv("CHARLY_CACHE_DIR", t.TempDir())
		got, err := StoreRoot()
		if err != nil {
			t.Fatalf("StoreRoot: %v", err)
		}
		want, err := storeroot.CacheDir()
		if err != nil {
			t.Fatalf("storeroot.CacheDir: %v", err)
		}
		if got != want {
			t.Fatalf("under CHARLY_CACHE_DIR: StoreRoot = %q, resolver = %q", got, want)
		}
	})
}
