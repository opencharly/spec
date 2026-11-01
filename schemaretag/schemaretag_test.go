package schemaretag

import (
	"strings"
	"testing"
)

const generated = "package params\n\ntype Foo struct {\n" +
	"\tName string `json:\"name,omitempty\"`\n" +
	"\tKind string `json:\"kind\"`\n" +
	"\tSkip string `yaml:\"already\" json:\"already\"`\n" +
	"\tOpt  string `yaml:\"opt,omitempty\" json:\"opt\"`\n" +
	"}\n"

// TestNormalizeDoublesJsonTagsAndAddsOmitempty pins the two transforms a
// gengotypes output needs, in the order the pipeline applies them. The expected
// strings are the transform's MEASURED output, not a guess — the same two passes
// reproduce a real committed `params/cue_types_gen.go` byte-identically (proved
// against candy/plugin-sidecar's file while landing opencharly/plugin-sidecar#8):
//   - a json-only tag is doubled with the yaml tag carrying the json value VERBATIM
//     (so `json:"name,omitempty"` yields `yaml:"name,omitempty" json:"name,omitempty"`),
//   - then any yaml key still BARE (no comma) gains `,omitempty` — which is why an
//     already-doubled `yaml:"kind" json:"kind"` comes out `yaml:"kind,omitempty" json:"kind"`,
//     and why a second pass changes nothing (every key now carries a comma).
func TestNormalizeDoublesJsonTagsAndAddsOmitempty(t *testing.T) {
	got := string(Normalize([]byte(generated)))
	for _, want := range []string{
		"`yaml:\"name,omitempty\" json:\"name,omitempty\"`",
		"`yaml:\"kind,omitempty\" json:\"kind\"`",
		"`yaml:\"already,omitempty\" json:\"already\"`",
		"`yaml:\"opt,omitempty\" json:\"opt\"`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Normalize output is missing %s\n--- got ---\n%s", want, got)
		}
	}
}

// TestNormalizeIsIdempotent is the property the pipeline depends on: re-running
// the whole pipeline over an already-normalized file must not churn it, so a
// clean regeneration is a no-op.
func TestNormalizeIsIdempotent(t *testing.T) {
	once := Normalize([]byte(generated))
	twice := Normalize(once)
	if string(once) != string(twice) {
		t.Fatalf("Normalize is not idempotent\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	// The `already`-normalized arms must be untouched by the first pass too: a
	// yaml tag that is not preceded by a bare json backtick must keep its options.
	if strings.Contains(string(once), `,omitempty,omitempty`) {
		t.Fatalf("Normalize appended a second ,omitempty:\n%s", once)
	}
}

// TestNormalizeLeavesNonTagTextAlone keeps the transform honest: it is a tag
// rewrite, not a text mangler.
func TestNormalizeLeavesNonTagTextAlone(t *testing.T) {
	src := []byte("// json:\"not-a-tag\" in a comment\nvar x = `json:\"\"`\n")
	got := string(Normalize(src))
	if !strings.Contains(got, "// json:\"not-a-tag\" in a comment") {
		t.Errorf("a comment mentioning a json tag was rewritten:\n%s", got)
	}
}
