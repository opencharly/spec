package spec

import (
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"gopkg.in/yaml.v3"

	"github.com/opencharly/spec/schemaconcat"
)

// loadStageParallelSchema compiles the SAME concatenation the runtime and `task cue:gen` use
// (schemaconcat.ConcatSchema — R3: one concatenation contract), so this test exercises the
// schema as it actually ships rather than a hand-written excerpt of it.
func loadStageParallelSchema(t *testing.T) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(os.DirFS(".."), "schema", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return v
}

// TestOpStageModifierSchematized — Cutover C task 3 validation: the `stage:` shared step
// modifier is TYPED (a dot-free string) in CUE (`=~"^[^.]+$"`). gengotypes renders it as a
// plain Go `string`, so only the CUE level can see the constraint — a Go-level round-trip
// test alone would pass identically with the regex reverted. Unifying concrete step values
// against the compiled #Step def is the same judgement the loader makes on an authored plan.
func TestOpStageModifierSchematized(t *testing.T) {
	schema := loadStageParallelSchema(t)

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		{"dot-free stage accepted", `{run: "x", stage: "setup"}`, false},
		{"hyphenated stage accepted", `{run: "x", stage: "warm-up"}`, false},
		{"check step with stage accepted", `{check: "x", stage: "verify"}`, false},
		{"plugin step with stage accepted", `{plugin: "x", stage: "probe"}`, false},
		{"dotted stage rejected", `{run: "x", stage: "a.b"}`, true},
		{"empty stage rejected", `{run: "x", stage: ""}`, true},
		{"absent stage still accepted", `{run: "x"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := schema.LookupPath(cue.ParsePath("#Step"))
			if err := def.Err(); err != nil {
				t.Fatalf("lookup #Step: %v", err)
			}
			val := schema.Context().CompileString(tc.value)
			if err := val.Err(); err != nil {
				t.Fatalf("compile value: %v", err)
			}
			unified := def.Unify(val)
			got := unified.Validate(cue.Concrete(false), cue.Final())
			if tc.reject && got == nil {
				t.Errorf("ACCEPTED %s — the stage modifier is not schematized", tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("rejected a valid stage value %s: %v", tc.value, got)
			}
			if tc.reject && got != nil && !strings.Contains(got.Error(), "stage") {
				t.Logf("note: rejection message does not name the stage field: %v", got)
			}
		})
	}
}

// TestDeployParallelScalarTyped — Cutover C task 3 validation: `parallel:` is a SUBSTRATE
// scalar (beside `disposable:`) and must be a bool on the authored deploy shape (#DeployValue).
// CUE is the only type-level enforcement (gengotypes renders `Parallel bool`); the schema
// gate is exercised here with accept/reject cases.
func TestDeployParallelScalarTyped(t *testing.T) {
	schema := loadStageParallelSchema(t)

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		{"parallel true accepted", `{parallel: true}`, false},
		{"parallel false accepted", `{parallel: false}`, false},
		{"parallel absent accepted", `{}`, false},
		{"parallel non-bool rejected", `{parallel: "yes"}`, true},
		{"parallel int rejected", `{parallel: 1}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := schema.LookupPath(cue.ParsePath("#DeployValue"))
			if err := def.Err(); err != nil {
				t.Fatalf("lookup #DeployValue: %v", err)
			}
			val := schema.Context().CompileString(tc.value)
			if err := val.Err(); err != nil {
				t.Fatalf("compile value: %v", err)
			}
			unified := def.Unify(val)
			got := unified.Validate(cue.Concrete(false), cue.Final())
			if tc.reject && got == nil {
				t.Errorf("ACCEPTED %s — parallel is not typed", tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("rejected a valid parallel value %s: %v", tc.value, got)
			}
			if tc.reject && got != nil && !strings.Contains(got.Error(), "parallel") {
				t.Logf("note: rejection message does not name the parallel field: %v", got)
			}
		})
	}
}

// TestOpStageYAMLRoundTrip pins the wire shape of the new generated fields: `stage:` rides
// the step Op and `parallel:` the DeployNode, both yaml-tagged, so authored configs
// decode/encode losslessly (B12 coverage for the schema addition).
func TestOpStageYAMLRoundTrip(t *testing.T) {
	body := `stage: setup
run: true-marker
`
	var op Op
	if err := yaml.Unmarshal([]byte(body), &op); err != nil {
		t.Fatalf("unmarshalling step Op: %v", err)
	}
	if op.Stage != "setup" {
		t.Fatalf("Op.Stage did not parse: got %q, want %q", op.Stage, "setup")
	}
	out, err := yaml.Marshal(&op)
	if err != nil {
		t.Fatalf("re-marshalling: %v", err)
	}
	var back Op
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshalling re-marshalled Op: %v", err)
	}
	if back.Stage != "setup" {
		t.Fatalf("Op.Stage did not survive the round trip: got %+v", back)
	}
}

// TestDeployNodeParallelYAMLRoundTrip — the parallel substrate scalar survives
// yaml -> Go -> yaml on the DeployNode (the type the bed runner consumes).
func TestDeployNodeParallelYAMLRoundTrip(t *testing.T) {
	body := `target: pod
parallel: true
disposable: true
`
	var node DeployNode
	if err := yaml.Unmarshal([]byte(body), &node); err != nil {
		t.Fatalf("unmarshalling DeployNode: %v", err)
	}
	if !node.Parallel {
		t.Fatalf("DeployNode.Parallel did not parse: got %+v", node)
	}
	out, err := yaml.Marshal(&node)
	if err != nil {
		t.Fatalf("re-marshalling: %v", err)
	}
	var back DeployNode
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshalling re-marshalled DeployNode: %v", err)
	}
	if !back.Parallel {
		t.Fatalf("DeployNode.Parallel did not survive the round trip: got %+v", back)
	}
}
