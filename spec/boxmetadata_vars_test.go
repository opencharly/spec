package spec

// boxmetadata_vars_test.go — a candy's declared `var:` must ROUND-TRIP onto BoxMetadata.
//
// This is the test that fails if the field is removed. `TestGenReproducible` cannot
// serve: it checks CUE↔Go consistency and passes identically whether the field exists
// or not, because a field absent from BOTH sides is still consistent. Serialization is
// runtime behaviour, so it needs a runtime assertion.
//
// The defect it guards: a candy's `#CandyFields.var` (schema/candy.cue) declares
// template variables, and `#Op`'s `config:` comment states the rule they follow —
// substitution is "candy var: union auto-exports". A `check:` step's substitution base
// is sdk/kit's buildTimeVars, which reads nine fields off BoxMetadata. While `var` was
// not one of them, a bed could declare a variable the runner KNEW by name (its
// unresolved classifier sees the declaration) and could never SUPPLY a value for —
// a SKIP rather than a DEAD error (opencharly/charly#865).

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestBoxMetadataCarriesCandyVars pins that an authored `var:` block reaches the Go
// field the check runner reads. Without it, a bed's declared variable decodes to a nil
// map and the runner has nothing to substitute.
func TestBoxMetadataCarriesCandyVars(t *testing.T) {
	const authored = `
var:
  CHARLY_PLUGIN_ONLY: "1"
  CHARLY_PLUGIN_DIR: /tmp/noplug
  PROBE_FLAG: from-the-candy-var
`
	var bm BoxMetadata
	if err := yaml.Unmarshal([]byte(authored), &bm); err != nil {
		t.Fatalf("decoding an authored var: block: %v", err)
	}

	// The FIELD must exist and be populated. A missing field would decode to nil here,
	// which is the whole regression this test exists to catch.
	if len(bm.Vars) != 3 {
		t.Fatalf("BoxMetadata.Vars has %d entries (%v), want the 3 the document authored — "+
			"a nil map means the field is missing from the shape, and a check step's "+
			"${NAME} substitution has nothing to resolve against", len(bm.Vars), bm.Vars)
	}
	for want := range map[string]string{
		"CHARLY_PLUGIN_ONLY": "1",
		"CHARLY_PLUGIN_DIR":  "/tmp/noplug",
		"PROBE_FLAG":         "from-the-candy-var",
	} {
		if _, ok := bm.Vars[want]; !ok {
			t.Errorf("%s did not survive the round-trip; got %v", want, bm.Vars)
		}
	}

	// The exact values, not just the keys: a field wired to the wrong YAML tag would
	// decode an empty map, and one wired with the wrong coercion would mangle "1".
	if got := bm.Vars["CHARLY_PLUGIN_ONLY"]; got != "1" {
		t.Errorf("CHARLY_PLUGIN_ONLY = %q, want %q — an unquoted YAML scalar must still "+
			"decode as its string form", got, "1")
	}
}

// TestBoxMetadataVarsRoundTrip pins the ENCODE side too: the tags must be `var`, so a
// BoxMetadata written back out is still authored in the shape a candy declares.
func TestBoxMetadataVarsRoundTrip(t *testing.T) {
	bm := BoxMetadata{Vars: map[string]string{"PROBE_FLAG": "x"}}
	out, err := yaml.Marshal(&bm)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	var back map[string]any
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-decoding our own output: %v", err)
	}
	vars, ok := back["var"].(map[string]any)
	if !ok {
		t.Fatalf("re-encoded BoxMetadata has no top-level `var:` key — the yaml tag is not "+
			"`var`, so a written-back manifest would use a spelling no candy declares: %s", out)
	}
	if vars["PROBE_FLAG"] != "x" {
		t.Errorf("var.PROBE_FLAG = %v, want x", vars["PROBE_FLAG"])
	}
}
