package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// stepConfig compiles the shipped schema and checks the given #Step src
// against #Step (the run arm embeds the CLOSED #Op), returning the error
// when the authored step does not satisfy it.
func stepConfig(t *testing.T, src string) error {
	t.Helper()
	conc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(conc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	step := v.LookupPath(cue.ParsePath("#Step"))
	if !step.Exists() {
		t.Fatal("#Step is not defined in the shipped schema")
	}
	unified := step.Unify(ctx.CompileString(src))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(false))
}

// The config: verb accepts a destination, a content template, and the
// validate: modifier naming a vendored CUE format schema.
func TestConfigStepAccepted(t *testing.T) {
	err := stepConfig(t, `{
		run: "render the crabbox config"
		config: "/etc/crabbox.yaml"
		content: "provider: ${CBX_CFG_PROVIDER}\n"
		validate: "crabbox-yaml"
		mode: "0600"
		run_as: "user"
	}`)
	if err != nil {
		t.Fatalf("config step should satisfy #Step: %v", err)
	}
}

// The config verb discriminator is a scalar destination path, exactly like
// write: — no structured map is accepted.
func TestConfigStepRejectsMapValue(t *testing.T) {
	err := stepConfig(t, `{
		run: "render"
		config: {path: "/etc/crabbox.yaml"}
	}`)
	if err == nil {
		t.Fatal("config: with a map value must be rejected (scalar path only)")
	}
}

// #Op is CLOSED — an unknown key on a config step is a typo and must fail.
func TestConfigStepRejectsUnknownField(t *testing.T) {
	err := stepConfig(t, `{
		run: "render"
		config: "/etc/crabbox.yaml"
		bogus_key: 1
	}`)
	if err == nil {
		t.Fatal("config step with an unknown field must be rejected (closed #Op)")
	}
}
