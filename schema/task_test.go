package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/spec"
)

// compileShipped compiles the shipped schema/*.cue concatenation (the SAME
// contract the runtime sharedCueSchema uses) and returns the value.
func compileShipped(t *testing.T) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(src)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	return v
}

// TestTaskValue_ValidTask passes a realistic task (every field set, incl. a
// plan with a builtin verb, a plugin-verb step, and an include) through the
// host-side value gate def #TaskValue.
func TestTaskValue_ValidTask(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#TaskValue"))
	if !def.Exists() {
		t.Fatal("#TaskValue is not defined in the shipped schema")
	}
	body := `{
	description: "build the binary"
	dir: "/workspace"
	env: {CGO_ENABLED: "0"}
	vars: {VERSION: "1.2.3"}
	depends_on: ["tidy"]
	sources: ["**/*.go"]
	generates: ["bin/charly"]
	status: ["test -f bin/charly"]
	timeout: "30m"
	params: {N: {description: "n", default: 1, required: false}}
	plan: [
		{check: "go is present", command: "go version", context: ["deploy"]},
		{run: "run the build", command: "./build.sh"},
		{run: "plugin verb step", plugin: "command", plugin_input: {command: "true"}},
		{include: "tidy"},
	]
}`
	in := cuecontext.New().CompileString(body)
	if err := in.Unify(def).Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("valid task must satisfy #TaskValue: %v", err)
	}
}

// TestTaskValue_RejectsUnknownField proves the gate is CLOSED: a typo'd
// task-level field is rejected.
func TestTaskValue_RejectsUnknownField(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#TaskValue"))
	in := cuecontext.New().CompileString(`{description: "x", bogus_field: 1}`)
	if err := in.Unify(def).Validate(cue.Concrete(true)); err == nil {
		t.Fatal("a typo'd task field must be rejected by #TaskValue")
	}
}

// TestTaskValue_RejectsUnknownStepField proves a task's plan steps are typed
// against the CLOSED #Step/#Op — an unknown Op field is rejected.
func TestTaskValue_RejectsUnknownStepField(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#TaskValue"))
	in := cuecontext.New().CompileString(`{
	description: "x",
	plan: [{run: "bad step", command: "true", not_an_op_field: 1}],
}`)
	if err := in.Unify(def).Validate(); err == nil {
		t.Fatal("an unknown step field must be rejected by #Step/#Op")
	}
}

// TestTaskValue_RequiresDescription proves the ADE identity field is required
// under the CONCRETE gate.
func TestTaskValue_RequiresDescription(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#TaskValue"))
	in := cuecontext.New().CompileString(`{dir: "/x"}`)
	if err := in.Unify(def).Validate(cue.Concrete(true)); err == nil {
		t.Fatal("a task missing description must fail the concrete gate")
	}
}

// TestKindValueDefs_HasTask gates the schemagen-derived word→def map: the new
// kind must be auto-derived into spec.KindValueDefs (the host gate consults it).
func TestKindValueDefs_HasTask(t *testing.T) {
	def, ok := spec.KindValueDefs["task"]
	if !ok || def != "#TaskValue" {
		t.Fatalf("KindValueDefs[task] = %q (ok=%v), want #TaskValue", def, ok)
	}
}
