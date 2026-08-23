package fleet

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// deploy_fleet_ops_candykey_test.go — CandyMapKey for the ROOT-LEVEL remote candy
// (the candy de-submodule cutover): a standalone candy repo's manifest lives at
// the repo root (SubPathPrefix ""), so the map key is the repo path itself —
// appending the name would double it. The sub-path form is unchanged. The
// embedded interface supplies the untouched methods (nil — any other call panics
// loudly rather than quietly changing what is under test).

type fakeCandy struct {
	spec.CandyReader
	name   string
	remote bool
	repo   string
	prefix string
}

func (f fakeCandy) GetName() string          { return f.name }
func (f fakeCandy) GetRemote() bool          { return f.remote }
func (f fakeCandy) GetRepoPath() string      { return f.repo }
func (f fakeCandy) GetSubPathPrefix() string { return f.prefix }

func TestCandyMapKey_RootLevelRemote(t *testing.T) {
	got := CandyMapKey(fakeCandy{name: "layer-ripgrep", remote: true, repo: "github.com/opencharly/layer-ripgrep"})
	if got != "github.com/opencharly/layer-ripgrep" {
		t.Fatalf("root-level remote key = %q, want the repo path itself", got)
	}
}

func TestCandyMapKey_SubPathRemote(t *testing.T) {
	got := CandyMapKey(fakeCandy{name: "ripgrep", remote: true, repo: "github.com/opencharly/charly", prefix: "candy/"})
	if got != "github.com/opencharly/charly/candy/ripgrep" {
		t.Fatalf("sub-path remote key = %q, want the full ref", got)
	}
}

func TestCandyMapKey_Local(t *testing.T) {
	got := CandyMapKey(fakeCandy{name: "ripgrep"})
	if got != "ripgrep" {
		t.Fatalf("local key = %q, want the bare name", got)
	}
}

// A root-level remote candy with an explicit name must STILL key as the repo
// path (the empty-prefix guard, not the name, decides the root-level shape).
func TestCandyMapKey_RootLevelRemoteNamed(t *testing.T) {
	got := CandyMapKey(fakeCandy{name: "layer-ripgrep", remote: true, repo: "github.com/opencharly/layer-ripgrep"})
	if got != "github.com/opencharly/layer-ripgrep" {
		t.Fatalf("root-level remote key = %q, want the repo path itself", got)
	}
}
