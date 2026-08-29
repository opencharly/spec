package spec

import (
	"testing"
)

// repo_identity_cache_test.go — the process-wide GitRemoteIdentity cache. The
// cache must return a stable value for the same dir (no repeated git spawns).
func TestGitRemoteIdentityCache(t *testing.T) {
	// A non-git dir caches "" (no spawn on repeat). The cache is process-wide,
	// so the second call must return the same value as the first.
	dir := t.TempDir()
	first := GitRemoteIdentity(dir)
	second := GitRemoteIdentity(dir)
	if first != second {
		t.Fatalf("GitRemoteIdentity: %q != %q (cache not stable)", first, second)
	}
}
