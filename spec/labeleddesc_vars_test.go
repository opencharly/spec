package spec

// labeleddesc_vars_test.go — the resolved description must carry a candy's declared vars.
//
// This is the leg that gives the check runner somewhere to READ a candy's `var:` from.
// BoxMetadata.Vars has a consumer (sdk's buildTimeVars) and no producer, because a
// container's metadata is decoded from OCI labels and `var:` is not one. The resolved
// description IS built from the project, so it is the shape where the declaration can
// actually travel.
//
// Serialization is runtime behaviour, so it needs a runtime assertion: TestGenReproducible
// checks CUE<->Go consistency and passes identically whether the field exists or not.

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLabeledDescriptionCarriesCandyVars pins that a `vars:` block authored on a resolved
// description reaches the Go field the check runner would read.
func TestLabeledDescriptionCarriesCandyVars(t *testing.T) {
	const authored = `
origin: candy/plugin-pipeline
vars:
  CHARACTER_SET: utf-8
  RETRIES: "3"
`
	var ld LabeledDescription
	if err := yaml.Unmarshal([]byte(authored), &ld); err != nil {
		t.Fatalf("decoding an authored description: %v", err)
	}
	if ld.Origin != "candy/plugin-pipeline" {
		t.Errorf("Origin = %q, want candy/plugin-pipeline — the required field must still decode", ld.Origin)
	}
	if len(ld.Vars) != 2 {
		t.Fatalf("LabeledDescription.Vars has %d entries (%v), want the 2 the document authored — "+
			"a nil map means the field is missing from the shape, and a check step's ${NAME} "+
			"has nothing to resolve against", len(ld.Vars), ld.Vars)
	}
	if got := ld.Vars["CHARACTER_SET"]; got != "utf-8" {
		t.Errorf("CHARACTER_SET = %q, want utf-8", got)
	}
	if got := ld.Vars["RETRIES"]; got != "3" {
		t.Errorf("RETRIES = %q, want 3 — a quoted numeric scalar stays a string", got)
	}
}

// TestLabeledDescriptionVarsRoundTrip pins the ENCODE side: the tag must be `vars`, so a
// resolved description written back out uses the spelling the shape declares.
func TestLabeledDescriptionVarsRoundTrip(t *testing.T) {
	ld := LabeledDescription{Origin: "candy/x", Vars: map[string]string{"FOO": "bar"}}
	out, err := yaml.Marshal(&ld)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	var back map[string]any
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-decoding our own output: %v", err)
	}
	vars, ok := back["vars"].(map[string]any)
	if !ok {
		t.Fatalf("re-encoded LabeledDescription has no top-level `vars:` key — the yaml tag is "+
			"not `vars`, so a written-back description would use a spelling no reader expects: %s", out)
	}
	if vars["FOO"] != "bar" {
		t.Errorf("vars.FOO = %v, want bar", vars["FOO"])
	}
}
