package spec

import (
	"os"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schemaconcat"
)

// TestDeployRecordWrap validates the deploy `record:` field (the whole-run
// recording wrap). It compiles the SAME concatenation the runtime uses
// (schemaconcat.ConcatSchema — R3), looks up #Deploy, and Unifies concrete
// values against it: a valid wrap (`record: {terminal: true}`) must be
// ACCEPTED, and an invalid one (`record: {fps: 0}` — below the >=1 bound) must
// be REJECTED. Both cases FAIL if the record: field / #RecordWrap def is
// missing from the schema (B12 coverage for the addition; the consuming runner
// + its bed R10 land in the plugin-check follow-up).
func TestDeployRecordWrap(t *testing.T) {
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
		{"record terminal wrap accepted", `{record: {terminal: true}}`, false},
		{"record desktop wrap accepted", `{record: {desktop: true, fps: 10, record_env: {XDG_RUNTIME_DIR: "/run/user/1000"}}}`, false},
		{"record fps below bound rejected", `{record: {fps: 0}}`, true},
		{"record unknown field rejected", `{record: {bogus: 1}}`, true},
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
