package refs

// cache_root_test.go — the operator's repo-cache overrides.
//
// Operator direction (2026-11-20): no CHARLY_ environment variable is dropped, so
// `CHARLY_REPO_CACHE` keeps its long-standing meaning — the REPOS DIRECTORY, used
// verbatim. When it is unset, the repos entry under the one cache root is used
// (`CHARLY_CACHE_DIR/repos`, else `$XDG_CACHE_HOME/charly/repos`, D-STORE-1).
//
// FOUR ARMS, because each answers a different claim: the variable's own meaning (and its
// verbatim relative value), the cache root's repos entry, and the resolver's default.

import (
	"path/filepath"
	"testing"

	"github.com/opencharly/spec/storeroot"
)

func TestRepoCacheDirHonorsTheOperatorOverrides(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("CHARLY_CACHE_DIR", root)

	// Arm 1: CHARLY_REPO_CACHE names the repos directory and wins over the cache root.
	repoCache := filepath.Join(home, "repo-cache")
	t.Setenv("CHARLY_REPO_CACHE", repoCache)
	got, err := RepoCacheDir()
	if err != nil {
		t.Fatalf("RepoCacheDir: %v", err)
	}
	if got != repoCache {
		t.Fatalf("RepoCacheDir = %q, want the CHARLY_REPO_CACHE value %q", got, repoCache)
	}

	// Arm 2: unset, the cache root's repos entry.
	t.Setenv("CHARLY_REPO_CACHE", "")
	got, err = RepoCacheDir()
	if err != nil {
		t.Fatalf("RepoCacheDir (root): %v", err)
	}
	if want := filepath.Join(root, "repos"); got != want {
		t.Fatalf("RepoCacheDir = %q, want %q", got, want)
	}

	// Arm 3: both overrides unset, the contract resolver's default.
	t.Setenv("CHARLY_CACHE_DIR", "")
	rootDir, err := storeroot.Dir(storeroot.RootCache)
	if err != nil {
		t.Fatalf("storeroot.Dir(RootCache): %v", err)
	}
	got, err = RepoCacheDir()
	if err != nil {
		t.Fatalf("RepoCacheDir (default): %v", err)
	}
	if want := filepath.Join(rootDir, "repos"); got != want {
		t.Fatalf("RepoCacheDir = %q, want %q — the two resolvers disagree on the DEFAULT", got, want)
	}

	// Arm 4: a relative value is used verbatim, as it always was.
	t.Setenv("CHARLY_REPO_CACHE", "relative-repos")
	got, err = RepoCacheDir()
	if err != nil {
		t.Fatalf("RepoCacheDir (relative): %v", err)
	}
	if got != "relative-repos" {
		t.Fatalf("RepoCacheDir = %q, want the relative value verbatim", got)
	}
}
