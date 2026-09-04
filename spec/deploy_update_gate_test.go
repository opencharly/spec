package spec

import (
	"os"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schemaconcat"
)

// TestDeployUpdateGate validates the check-bed `update_gate:` field (the R10
// fresh-update change-class switch). It compiles the SAME concatenation the
// runtime uses (schemaconcat.ConcatSchema — R3), looks up #Deploy, and Unifies
// concrete values against it: the closed enum's three legal values (+ the
// defaulted absence) must be ACCEPTED, and a value outside the enum must be
// REJECTED. Both cases FAIL if the update_gate field is missing from the schema
// (B12 coverage for the addition; the runner behavior + its unit tests land in
// the plugin-check follow-up).
func TestDeployUpdateGate(t *testing.T) {
	src, _, err := schemaconcat.ConcatSchema(os.DirFS(".."), "schema", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	ctx := cuecontext.New()
	schema := ctx.CompileString(src)
	if err := schema.Err(); err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	def := schema.LookupPath(cue.ParsePath("#Deploy"))
	if err := def.Err(); err != nil {
		t.Fatalf("lookup #Deploy: %v", err)
	}

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		{"update_gate default is full", `{}`, false},
		{"update_gate full accepted", `{update_gate: "full"}`, false},
		{"update_gate restart-only accepted", `{update_gate: "restart-only"}`, false},
		{"update_gate skip accepted", `{update_gate: "skip"}`, false},
		{"update_gate unknown value rejected", `{update_gate: "rebuild"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := def.Unify(ctx.CompileString(tc.value))
			err := v.Validate()
			if tc.reject && err == nil {
				t.Fatalf("value %s was accepted, want rejection: %v", tc.value, v)
			}
			if !tc.reject && err != nil {
				t.Fatalf("value %s rejected: %v", tc.value, err)
			}
		})
	}
}
