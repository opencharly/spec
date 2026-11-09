package specmodule_test

import (
	"io/fs"
	"testing"

	"cuelang.org/go/mod/modfile"

	specmodule "github.com/opencharly/spec"
)

// TestFSDeclaresTheCueModule gates the ONE fact a Go consumer cannot derive from
// `schema.FS`: that the schema is a package in a CUE module, and WHICH language
// version that module pins. The consumer composes the module instance from these
// two embedded trees, so a missing file, an unparseable declaration, or a language
// version that silently changed all have to fail HERE rather than as a schema that
// evaluates under different semantics in the consumer.
func TestFSDeclaresTheCueModule(t *testing.T) {
	var found []string
	if err := fs.WalkDir(specmodule.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = append(found, p)
		}
		return nil
	}); err != nil {
		t.Fatalf("walking the module FS: %v", err)
	}
	if len(found) != 1 || found[0] != "cue.mod/module.cue" {
		t.Fatalf("the module FS carries %v, want exactly [cue.mod/module.cue] — a consumer overlays this tree at its module root, so an extra or misnamed file would land somewhere the loader does not read", found)
	}

	data, err := fs.ReadFile(specmodule.FS, "cue.mod/module.cue")
	if err != nil {
		t.Fatalf("reading cue.mod/module.cue: %v", err)
	}
	f, err := modfile.ParseNonStrict(data, "cue.mod/module.cue")
	if err != nil {
		t.Fatalf("cue.mod/module.cue does not parse as a CUE module file: %v", err)
	}
	if got, want := f.Module, "github.com/opencharly/spec"; got != want {
		t.Fatalf("the module file declares module %q, want %q — a consumer resolves the schema package against this path", got, want)
	}
	if f.Language == nil || f.Language.Version == "" {
		t.Fatal("the module file declares no language.version — the schema does NOT evaluate under the toolchain's default semantics: its close() + embedded-definition idiom (#Op's arms) is legal through v0.17 and rejected by v0.18's")
	}
	if got, want := f.Language.Version, "v0.16.1"; got != want {
		t.Fatalf("the module pins language version %q, want %q — this is a SEMANTIC pin, not a toolchain bump: raising it is the §10.1 pattern rewrite (TODO 2.16), and a consumer composing this module would otherwise evaluate the schema under different semantics", got, want)
	}
}
