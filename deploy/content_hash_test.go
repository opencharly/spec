package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCandy creates a candy source tree with the given files and returns its dir.
func writeCandy(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// TestCandyStageDirName_SameContentSameName is the regression witness for the ref-change layer
// invalidation: the SAME candy content at two different versions must stage under ONE directory,
// so a ref-only change keeps the Containerfile's COPY source (and its BuildKit layer key) stable.
// A version-keyed name made the COPY path change with every ref, re-executing the pixi stage.
func TestCandyStageDirName_SameContentSameName(t *testing.T) {
	files := map[string]string{
		"pixi.toml": "[workspace]\nname = \"x\"\n",
		"pixi.lock": "version: 1\n",
	}
	dirA := writeCandy(t, files)
	dirB := writeCandy(t, files) // a second checkout of identical content

	a := CandyStageDirName(fakeCandy{name: "demo", version: "v2026.1.1", sourceDir: dirA})
	b := CandyStageDirName(fakeCandy{name: "demo", version: "v2026.2.2", sourceDir: dirB})
	if a != b {
		t.Fatalf("identical content at two versions staged under different dirs: %q vs %q", a, b)
	}
	if a == "demo.v2026.1.1" || a == "demo.v2026.2.2" {
		t.Fatalf("the staging name is still version-keyed: %q", a)
	}
}

// TestCandyStageDirName_DifferentContentDifferentName: changed content must stage under a
// different directory, or two versions would clobber each other's files.
func TestCandyStageDirName_DifferentContentDifferentName(t *testing.T) {
	dirA := writeCandy(t, map[string]string{"pixi.toml": "a\n"})
	dirB := writeCandy(t, map[string]string{"pixi.toml": "b\n"})
	a := CandyStageDirName(fakeCandy{name: "demo", version: "v1", sourceDir: dirA})
	b := CandyStageDirName(fakeCandy{name: "demo", version: "v2", sourceDir: dirB})
	if a == b {
		t.Fatalf("different content staged under the same dir: %q", a)
	}
}

// TestCandyStageDirName_FallsBackToVersion: a layer with no resolved source dir (or an
// unreadable one) keeps the version-keyed name, so a caller never stages under an address it
// could not derive.
func TestCandyStageDirName_FallsBackToVersion(t *testing.T) {
	if got := CandyStageDirName(fakeCandy{name: "demo", version: "v1"}); got != "demo.v1" {
		t.Fatalf("no source dir: got %q, want demo.v1", got)
	}
	if got := CandyStageDirName(fakeCandy{name: "demo"}); got != "demo" {
		t.Fatalf("no version: got %q, want demo", got)
	}
}
