package spec

import (
	"reflect"
	"testing"
)

// Compile-time pin of the override-precedence seam (charly #587's named follow-up wave):
// this METHOD EXPRESSION type-checks ONLY IF ProjectLoader declares RepoOverrideDir with
// exactly this signature — drop the method or change the shape and the spec package fails
// to build. It is the interface analogue of TestRefsCollectSeams_LatestTagSeam's
// compiles-only-if contract, for the seam that carries loaderkit's ONE CHARLY_REPO_OVERRIDE
// parse (sdk #251) to charly core, which may not import the sdk
// (charly/import_purity_test.go) and reaches it through requireProjectLoader().
var _ func(ProjectLoader, string) (string, bool, error) = ProjectLoader.RepoOverrideDir

// TestProjectLoader_RepoOverrideDirSeam pins the seam signature that the two follow-on steps
// depend on: candy/plugin-loader implements it by delegating to loaderkit.RepoOverrideDir
// (reading the env value ITSELF — hence the ONE repoPath parameter), and charly core calls it
// through requireProjectLoader() before its baked-artifact search. A caller cannot thread its
// own env read through this seam: a second parse anywhere is the R3 divergence this seam
// exists to prevent.
func TestProjectLoader_RepoOverrideDirSeam(t *testing.T) {
	m, ok := reflect.TypeOf((*ProjectLoader)(nil)).Elem().MethodByName("RepoOverrideDir")
	if !ok {
		t.Fatal("ProjectLoader must declare RepoOverrideDir(repoPath string) (string, bool, error)")
	}
	want := reflect.TypeOf(func(string) (string, bool, error) { return "", false, nil })
	if m.Type != want {
		t.Fatalf("RepoOverrideDir signature = %s, want %s (the env value is read implementation-side, so repoPath is the ONLY parameter)", m.Type, want)
	}
	if m.Type.NumIn() != 1 {
		t.Fatalf("RepoOverrideDir takes %d parameters, want 1 (repoPath)", m.Type.NumIn())
	}
}
