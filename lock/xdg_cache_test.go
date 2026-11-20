package lock

import (
	"os"
	"path/filepath"
	"testing"
)

// TestImageBuildLockPathFollowsTheCacheRoot is the R7 control for the lock-path
// migration: the pre-change code called os.UserCacheDir(), which ignores
// CHARLY_CACHE_DIR, so the lock landed under $HOME/.cache/charly; the resolver puts
// it under the one cache root. The test fails on the pre-change tree.
func TestImageBuildLockPathFollowsTheCacheRoot(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "relocated-cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("CHARLY_CACHE_DIR", override)

	path, err := ImageBuildLockPath("ghcr.io/opencharly/example:2026.1")
	if err != nil {
		t.Fatalf("ImageBuildLockPath: %v", err)
	}
	if got, want := filepath.Dir(path), filepath.Join(override, "locks"); got != want {
		t.Errorf("lock dir = %q, want the cache root's locks dir %q", got, want)
	}
}

// TestAcquireVmDomainLockFollowsTheCacheRoot is the R7 control for the bed-domain
// lock: the pre-change code used $HOME/.cache/charly/.locks and ignored the
// override, so the lock file is absent under the override on that tree.
func TestAcquireVmDomainLockFollowsTheCacheRoot(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "relocated-cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("CHARLY_CACHE_DIR", override)

	release, err := AcquireVmDomainLock("xdg-test-domain")
	if err != nil {
		t.Fatalf("AcquireVmDomainLock: %v", err)
	}
	defer func() { _ = release() }()

	path := filepath.Join(override, ".locks", "vm-domain-xdg-test-domain.lock")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file %s is not under the cache root: %v", path, err)
	}
}
