package refs

// cache_root_test.go — D-STORE-1's "single override" clause, on the one place this module kept a
// second one.
//
// DESIGN.md:2482 (D-STORE-1): the roots are resolved by ONE resolver on the contract surface
// (`spec/storeroot`), cache is `$XDG_CACHE_HOME/charly` with the SINGLE override `CHARLY_CACHE_DIR`.
// `RepoCacheDir` read a second override (`CHARLY_REPO_CACHE`) and then hand-built
// `$HOME/.cache/charly/repos` — both forbidden by the same row, and the second one is why the two
// resolvers could disagree.
//
// TWO ARMS, because they answer two different claims:
//   - the override arm: `CHARLY_CACHE_DIR` governs, and the retired variable does not;
//   - the default arm: with every override unset the answer equals what the contract resolver gives,
//     which is an EQUIVALENCE between two resolvers and therefore needs an instrument, not a sentence.

import (
	"path/filepath"
	"testing"

	"github.com/opencharly/spec/storeroot"
)

func TestRepoCacheDirIsGovernedByTheOneResolver(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("CHARLY_CACHE_DIR", root)
	// The retired second override, pointed somewhere the ONE resolver can never produce.
	t.Setenv("CHARLY_REPO_CACHE", filepath.Join(home, "retired-repo-cache"))

	got, err := RepoCacheDir()
	if err != nil {
		t.Fatalf("RepoCacheDir: %v", err)
	}
	if want := filepath.Join(root, "repos"); got != want {
		t.Fatalf("RepoCacheDir = %q, want %q — CHARLY_REPO_CACHE is a second override, and D-STORE-1 admits exactly one (CHARLY_CACHE_DIR)", got, want)
	}

	// The default arm: no override anywhere, and the answer must equal the contract resolver's.
	t.Setenv("CHARLY_CACHE_DIR", "")
	t.Setenv("CHARLY_REPO_CACHE", "")
	rootDir, err := storeroot.Dir(storeroot.RootCache)
	if err != nil {
		t.Fatalf("storeroot.Dir(RootCache): %v", err)
	}
	got, err = RepoCacheDir()
	if err != nil {
		t.Fatalf("RepoCacheDir (default): %v", err)
	}
	if want := filepath.Join(rootDir, "repos"); got != want {
		t.Fatalf("RepoCacheDir = %q, want %q — the two resolvers disagree on the DEFAULT, so a host that set nothing would be moved", got, want)
	}
}
