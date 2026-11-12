package schema_test

// compile_shipped_test.go — the shared fixture the schema package's tests build on: the
// SHIPPED `schema/*.cue` concatenation compiled to a CUE value, i.e. the same contract the
// runtime's sharedCueSchema uses.
//
// It lived in `task_test.go` until opencharly/spec#304 unit 2 deleted that file (the `task`
// body moved to its own plugin); several other tests in this package already used it, so it
// moved here rather than going with the corpus that no longer needs it.

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
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
