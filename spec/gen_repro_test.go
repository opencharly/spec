package spec

// Reproducibility gate: the committed cue_types_gen.go and vocab_gen.go MUST
// equal a fresh `charly task cue-gen`. This re-runs the SAME tools the task runs (the
// internal/schemagen concat + the in-process schemaparams generator + the
// schemagen vocab emitter) into a temp dir and compares the result BYTE FOR BYTE
// against the committed files. Nothing here needs the `cue` CLI — generation is
// in-process — so the gate never skips, and it catches any drift between
// schema/*.cue and the committed generated Go.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// freshTypesGen reproduces spec/cue_types_gen.go through the ONE pipeline: the same
// `schemagen -mode=params` the `cue-gen` task runs, which delegates to
// schemaparams.GenerateExcluding. The test does not re-run the individual steps —
// concatenation, composition, generation and the tag normalization all live in that
// one function now, and a test that reproduced them itself would be a second copy of
// the mechanism it is meant to gate (and would not notice the task's path breaking).
func freshTypesGen(t *testing.T) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "cue_types_gen.go")
	runIn(t, "..", "go", "run", "./internal/schemagen", "-mode=params", "-schema=schema", "-pkg=spec", "-out="+out)
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read fresh types: %v", err)
	}
	return raw
}

// freshVocabGen reproduces spec/vocab_gen.go via schemagen -mode=vocab.
func freshVocabGen(t *testing.T) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "vocab_gen.go")
	runIn(t, "..", "go", "run", "./internal/schemagen", "-mode=vocab", "-schema=schema", "-out="+out)
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read fresh vocab: %v", err)
	}
	return raw
}

func runIn(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func committed(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read committed %s: %v", name, err)
	}
	return raw
}

func TestGenReproducible(t *testing.T) {
	if got, want := freshTypesGen(t), committed(t, "cue_types_gen.go"); !equalBytes(got, want) {
		t.Errorf("cue_types_gen.go is STALE: a fresh `charly task cue-gen` differs from the committed file.\n"+
			"Run `charly task cue-gen` and commit the result. (fresh=%d bytes, committed=%d bytes)", len(got), len(want))
	}
	if got, want := freshVocabGen(t), committed(t, "vocab_gen.go"); !equalBytes(got, want) {
		t.Errorf("vocab_gen.go is STALE: a fresh `charly task cue-gen` differs from the committed file.\n"+
			"Run `charly task cue-gen` and commit the result. (fresh=%d bytes, committed=%d bytes)", len(got), len(want))
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
