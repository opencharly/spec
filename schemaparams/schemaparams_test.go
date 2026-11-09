package schemaparams

// schemaparams_test.go — the pipeline's contract.
//
// Three kinds of arm, each proving a different claim:
//
//   - the IMPORT case (opencharly/spec#298, F1+F2): a plugin schema carrying
//     `import "github.com/opencharly/spec/schema"` generates params that reference the
//     Go package that OWNS the shapes, and the generated file COMPILES against the
//     contract module (a real `go build`, not a string assertion);
//   - the NO-IMPORT case: output must stay byte-identical to the pinned
//     `cue exp gengotypes`, which the oracle arm proves by running that CLI on the same
//     source (it skips visibly when the pinned binary cannot be provisioned);
//   - the pipeline's own edges (package name, file filter, empty dir, unregistered
//     import).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/cuetoolchain"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/schemaretag"
)

// TestGenerateNoImportIsByteIdenticalToGengotypes is the no-regression proof: the
// in-process generator must produce EXACTLY what the pinned `cue exp gengotypes` produces
// for a schema without an import (the twin shape, every plugin's shape today). The oracle
// re-runs the old recipe verbatim — concat, scratch module, CLI, tag normalization — so
// the comparison is against the real toolchain, never a hand-written expectation.
func TestGenerateNoImportIsByteIdenticalToGengotypes(t *testing.T) {
	cue := pinnedCue(t)
	if cue == "" {
		t.Skipf("pinned cue %s could not be provisioned here — the byte-identity oracle is skipped, not faked", cuetoolchain.Version)
	}
	dir := filepath.Join("testdata", "twin", "schema")
	want := gengotypesOracle(t, dir, "params", cue)
	got, err := Generate(dir, "params")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("in-process output differs from `cue exp gengotypes` (%d vs %d bytes)\n--- gengotypes ---\n%s\n--- in-process ---\n%s", len(want), len(got), want, got)
	}
}

// TestGenerateImportReferencesTheOwningGoPackage is F2: the emitted Go must import the
// package that owns the shapes (`github.com/opencharly/spec/spec`), never the CUE
// package path whose Go package exports only FS.
func TestGenerateImportReferencesTheOwningGoPackage(t *testing.T) {
	out, err := Generate(filepath.Join("testdata", "import", "schema"), "params")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "\"github.com/opencharly/spec/spec\"") {
		t.Fatalf("generated file does not import the Go package that owns the shapes:\n%s", src)
	}
	if strings.Contains(src, "\"github.com/opencharly/spec/schema\"") {
		t.Fatalf("generated file imports the CUE package path as Go (F2):\n%s", src)
	}
	if !strings.Contains(src, "Config spec.TunnelConfig ") {
		t.Fatalf("generated field does not reference spec.TunnelConfig:\n%s", src)
	}
}

// TestGenerateImportCompiles proves the import case at its REAL boundary: the generated
// params file is placed in a module that requires this one and `go build` compiles it.
// A string assertion cannot catch a Go package that exists but lacks the type (the exact
// F2 failure), so this arm builds it.
func TestGenerateImportCompiles(t *testing.T) {
	out, err := Generate(filepath.Join("testdata", "import", "schema"), "params")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	// The temp module mirrors THIS module's declaration: the same go directive and the
	// same requirement set, with only the module path rewritten and the contract module
	// added as a local replace. A minimal `require spec` makes `go build` EXPAND the
	// module graph, which asks the proxy for go.mod files the repo's own build never
	// needs — measured in CI, where gopkg.in/check.v1's go.mod was absent from the cache
	// and GOPROXY=off refused the lookup. A complete declaration is also what a real
	// consumer of this contract module has.
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read the module's go.mod: %v", err)
	}
	gomod := strings.Replace(string(mod), "module github.com/opencharly/spec", "module example.com/pluginparams", 1)
	if gomod == string(mod) {
		t.Fatalf("the contract module's go.mod does not start with its module path")
	}
	gomod += "\nrequire github.com/opencharly/spec v0.0.0\n\nreplace github.com/opencharly/spec => " + root + "\n"
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatalf("read the module's go.sum (needed by the temp module's build): %v", err)
	}
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "params"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "params", "cue_types_gen.go"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.sum"), sum, 0o644); err != nil {
		t.Fatal(err)
	}
	// No GOFLAGS: the default readonly mode uses the complete declaration above and
	// never tries to rewrite it (a rewrite is what needed the proxy).
	cmd := exec.Command("go", "build", "./params")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the generated params do not compile against the contract module: %v\n%s\n--- generated ---\n%s", err, buildOut, out)
	}
}

// TestGenerateHonoursThePackageName: the header follows the caller's package, so the same
// pipeline serves a plugin (`params`) and spec itself (`spec`).
func TestGenerateHonoursThePackageName(t *testing.T) {
	dir := filepath.Join("testdata", "twin", "schema")
	for _, pkg := range []string{"params", "spec"} {
		out, err := Generate(dir, pkg)
		if err != nil {
			t.Fatalf("Generate(%q): %v", pkg, err)
		}
		if !strings.Contains(string(out), "\npackage "+pkg+"\n") {
			t.Fatalf("generated file does not declare package %q:\n%.120s", pkg, out)
		}
	}
}

// TestGenerateExcludingHonoursTheFilter: the file filter is the caller's only input
// difference (spec excludes node.cue), so an excluded file must contribute nothing.
func TestGenerateExcludingHonoursTheFilter(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.cue": "#A: {\n\tname?: string\n}\n",
		"b.cue": "#B: {\n\tother?: string\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := GenerateExcluding(dir, "params", func(name string) bool { return name == "b.cue" })
	if err != nil {
		t.Fatalf("GenerateExcluding: %v", err)
	}
	if !strings.Contains(string(out), "type A struct") {
		t.Fatalf("the included definition is missing:\n%s", out)
	}
	if strings.Contains(string(out), "type B struct") {
		t.Fatalf("the EXCLUDED file still generated a type:\n%s", out)
	}
}

// TestGenerateRejectsAnEmptySchemaDir: "no schema" must be an error naming the directory,
// never an empty generated file.
func TestGenerateRejectsAnEmptySchemaDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Generate(dir, "params")
	if err == nil || !strings.Contains(err.Error(), "no *.cue files") {
		t.Fatalf("want an empty-schema error, got %v", err)
	}
}

// TestGoImportForRejectsAnUnregisteredPackage: an import the projection cannot name a Go
// home for is a loud error, never a reference to a Go path that may not compile.
func TestGoImportForRejectsAnUnregisteredPackage(t *testing.T) {
	g := &generator{goImports: map[string]goImport{}, importAliases: map[string]string{}}
	_, err := g.goImportFor("example.com/other/schema")
	if err == nil || !strings.Contains(err.Error(), "no Go package is registered") {
		t.Fatalf("want an unregistered-import error, got %v", err)
	}
}

// TestGoImportForUsesTheGoHome: the relation maps the CUE package to its Go owner, and an
// explicit CUE import alias is preserved as the Go qualifier.
func TestGoImportForUsesTheGoHome(t *testing.T) {
	g := &generator{goImports: map[string]goImport{}, importAliases: map[string]string{"github.com/opencharly/spec/schema": "contract"}}
	gi, err := g.goImportFor("github.com/opencharly/spec/schema")
	if err != nil {
		t.Fatal(err)
	}
	if gi.Path != "github.com/opencharly/spec/spec" || gi.Qualifier != "contract" {
		t.Fatalf("goImportFor = %+v, want path spec/spec and qualifier contract", gi)
	}
}

// pinnedCue resolves the pinned `cue` CLI for the oracle arm, provisioning it through the
// SAME cuetoolchain the repo's bootstrap task uses. "" means it cannot run here.
func pinnedCue(t *testing.T) string {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	bin, err := cuetoolchain.Ensure(filepath.Join(cache, "charly", "cue", cuetoolchain.Version))
	if err != nil {
		return ""
	}
	return bin
}

// gengotypesOracle reproduces the pipeline the shell-out used: concatenate the schema
// directory, head it `package <pkg>` + `@go(<pkg>)`, write a scratch CUE module, run the
// pinned `cue exp gengotypes`, normalize the tags. It is a TEST-ONLY oracle: the old
// recipe must be reproducible to compare against, but it is not product code any more.
func gengotypesOracle(t *testing.T, schemaDir, pkg, cueBin string) []byte {
	t.Helper()
	body, files, err := schemaconcat.ConcatSchema(os.DirFS(schemaDir), ".", nil)
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.cue files in %s", schemaDir)
	}
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	mod := "module: \"github.com/opencharly/spec/schemaparams/codegen\"\nlanguage: version: \"" + cuetoolchain.Version + "\"\n"
	if err := os.WriteFile(filepath.Join(tmp, "cue.mod", "module.cue"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "package " + pkg + "\n\n@go(" + pkg + ")\n\n" + body
	if err := os.WriteFile(filepath.Join(tmp, pkg+".cue"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(cueBin, "exp", "gengotypes", ".")
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cue exp gengotypes failed: %v\n%s", err, out)
	}
	gen, err := os.ReadFile(filepath.Join(tmp, "cue_types_"+pkg+"_gen.go"))
	if err != nil {
		t.Fatalf("gengotypes produced no output: %v", err)
	}
	return schemaretag.Normalize(gen)
}
