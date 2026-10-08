package schemaparams

// schemaparams_test.go — the pipeline's contract, exercised through the real code
// path with a STAND-IN toolchain (deterministic, no network) plus one arm that runs
// the REAL provisioned `cue`, which skips visibly wherever it cannot be provisioned.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/cuetoolchain"
)

// stubCue writes an executable standing in for `cue exp gengotypes`: it CAPTURES the
// CUE source it was handed (so the concat+header contract can be asserted) and emits
// a fixed generated file with a json-only tag (so the retag contract can be asserted).
func stubCue(t *testing.T, captureDir, body, exitLine string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cue")
	// The stub derives the generated file's name from the CUE file it is handed
	// (`<pkg>.cue`), exactly as the real `cue exp gengotypes` names its output —
	// so the same stub serves any package name.
	script := "#!/bin/sh\n" +
		"f=$(ls *.cue | head -1); pkg=$(basename \"$f\" .cue)\n" +
		"cp \"$f\" \"" + captureDir + "/captured.cue\" 2>/dev/null || true\n" +
		exitLine + "\n" +
		"cat > \"cue_types_${pkg}_gen.go\" <<'EOF'\n" + body + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func schemaDirWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "schema")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const stubBody = "package params\n\ntype Foo struct {\n\tName string `json:\"name\"`\n}"

// TestGenerateWithCueRunsConcatHeaderAndRetag pins all three steps in one arm: the
// schema files arrive concatenated under the `package params` + `@go(params)` header
// (captured from the child), and the returned file has the tags normalized.
func TestGenerateWithCueRunsConcatHeaderAndRetag(t *testing.T) {
	capture := t.TempDir()
	schema := schemaDirWith(t, map[string]string{
		"a.cue": "#A: {\n\tname?: string\n}\n",
		"b.cue": "#B: {\n\tother?: string\n}\n",
	})
	bin := stubCue(t, capture, stubBody, "true")

	got, err := GenerateWithCue(schema, "params", bin)
	if err != nil {
		t.Fatalf("GenerateWithCue: %v", err)
	}
	if want := "package params\n\ntype Foo struct {\n\tName string `yaml:\"name,omitempty\" json:\"name\"`\n}\n"; string(got) != want {
		t.Fatalf("pipeline output =\n%q\nwant\n%q", got, want)
	}

	handed, err := os.ReadFile(filepath.Join(capture, "captured.cue"))
	if err != nil {
		t.Fatalf("the toolchain was not handed a CUE source: %v", err)
	}
	src := string(handed)
	if !strings.HasPrefix(src, "package params\n\n@go(params)\n\n") {
		t.Fatalf("the concatenation is not headed by the package clause + @go attribute:\n%.120s", src)
	}
	for _, want := range []string{"#A: {", "#B: {"} {
		if !strings.Contains(src, want) {
			t.Fatalf("the concatenation is missing %s:\n%s", want, src)
		}
	}
	if strings.Index(src, "#A: {") > strings.Index(src, "#B: {") {
		t.Fatalf("the concatenation is not in sorted file order:\n%s", src)
	}
}

// TestGenerateWithCueHonoursThePackageName: the header follows the caller's package,
// so the same function serves a plugin (`params`) and spec itself (`spec`).
func TestGenerateWithCueHonoursThePackageName(t *testing.T) {
	capture := t.TempDir()
	schema := schemaDirWith(t, map[string]string{"a.cue": "#A: {\n\tname?: string\n}\n"})
	bin := stubCue(t, capture, "package spec\n\ntype Foo struct {\n\tName string `json:\"name\"`\n}", "true")

	if _, err := GenerateWithCue(schema, "spec", bin); err != nil {
		t.Fatalf("GenerateWithCue: %v", err)
	}
	handed, err := os.ReadFile(filepath.Join(capture, "captured.cue"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(handed), "package spec\n\n@go(spec)\n\n") {
		t.Fatalf("the header does not follow the caller's package:\n%.80s", handed)
	}
}

// TestGenerateWithCueRejectsAnEmptySchemaDir: "no schema" must be an error naming the
// directory, never an empty generated file.
func TestGenerateWithCueRejectsAnEmptySchemaDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := GenerateWithCue(dir, "params", stubCue(t, t.TempDir(), stubBody, "true"))
	if err == nil || !strings.Contains(err.Error(), "no *.cue files") {
		t.Fatalf("want an empty-schema error, got %v", err)
	}
}

// TestGenerateWithCueSurfacesAToolchainFailure: a failing generator is reported with
// its own output, not as an empty or partial result.
func TestGenerateWithCueSurfacesAToolchainFailure(t *testing.T) {
	schema := schemaDirWith(t, map[string]string{"a.cue": "#A: {\n\tname?: string\n}\n"})
	bin := stubCue(t, t.TempDir(), stubBody, "echo 'cue: boom' >&2; exit 1")
	_, err := GenerateWithCue(schema, "params", bin)
	if err == nil {
		t.Fatal("a failing toolchain produced no error")
	}
	if !strings.Contains(err.Error(), "gengotypes failed") || !strings.Contains(err.Error(), "cue: boom") {
		t.Fatalf("the failure must name the step and carry the toolchain's own output, got: %v", err)
	}
}

// TestGenerateProvisionsThePinnedToolchain runs the REAL pipeline: Generate resolves
// the pinned `cue` through cuetoolchain.Ensure (no stub), generates from a tiny schema
// and normalizes the tags. It needs the release to be provisionable, so it skips
// VISIBLY — never with a canned substitute — when that is impossible here.
func TestGenerateProvisionsThePinnedToolchain(t *testing.T) {
	schema := schemaDirWith(t, map[string]string{
		"a.cue": "// The leading doc comment, which gengotypes reproduces.\n#A: {\n\tname?: string\n\tretry_limit?: int\n}\n",
	})
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache dir to provision the toolchain into: %v", err)
	}
	got, err := Generate(schema, "params", filepath.Join(cache, "charly", "cue", cuetoolchain.Version))
	if err != nil {
		t.Skipf("the pinned cue toolchain could not be provisioned here (%v) — this arm is skipped, not faked", err)
	}
	src := string(got)
	if !strings.Contains(src, "package params") {
		t.Fatalf("generated source has no package clause:\n%.200s", src)
	}
	// The leading doc comment is reproduced by the generator — the trap that makes a
	// grep-for-a-retired-string check useless — and the tags are normalized.
	if !strings.Contains(src, "The leading doc comment") {
		t.Fatalf("the schema's leading doc comment did not survive into the generated file:\n%.300s", src)
	}
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "json:\"") && !strings.Contains(line, "yaml:\"") {
			t.Fatalf("a generated line carries a json tag with no yaml tag (retag did not run):\n%s", line)
		}
	}
}
