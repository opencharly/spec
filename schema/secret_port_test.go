package schema_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/spec"
)

// shippedSchema compiles the SHIPPED schema (schema.FS through schemaconcat, the same path the runtime
// and `charly task cue-gen` use) and returns its root value.
func shippedSchema(t *testing.T) cue.Value {
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

// TestAbsPathRefusesAVariable is the test this PR was missing on its first pass.
//
// The body called `[^$]` "load-bearing rather than cosmetic" and then never exercised it: a re-spelling
// of #AbsPath to `=~"^/.*$"` — reopening exactly the variable-expansion leak the comment names — would
// have passed every subtest. A claim about a constraint is worth nothing until a command refuses the
// violating input.
//
// It evaluates the SHIPPED schema, so it fails if the definition is re-spelled in schema/secret.cue.
func TestAbsPathRefusesAVariable(t *testing.T) {
	v := shippedSchema(t).LookupPath(cue.ParsePath("#AbsPath"))
	if !v.Exists() {
		t.Fatal("#AbsPath is not in the shipped schema — the port is missing")
	}
	ctx := cuecontext.New()

	// The allowed half: a literal absolute path.
	if err := v.Unify(ctx.CompileString(`"/run/charly/secrets/db_password"`)).Err(); err != nil {
		t.Fatalf("a literal absolute path must be allowed by #AbsPath: %v", err)
	}

	// The refused half, and the reason this test exists: `$DB_PASSWORD` would expand into a VALUE at
	// delivery — the leak D-SEC-1 closes, reopened through the path instead of the value.
	if err := v.Unify(ctx.CompileString(`"/run/secrets/$DB_PASSWORD"`)).Err(); err == nil {
		t.Fatal("#AbsPath accepted a path carrying `$` — a secret value could be smuggled through variable expansion (D-SEC-1)")
	}

	// Its sibling: a relative path is refused, because `dst` must land somewhere unambiguous.
	if err := v.Unify(ctx.CompileString(`"secrets/db"`)).Err(); err == nil {
		t.Fatal("#AbsPath accepted a RELATIVE path; dst must be absolute")
	}
}

// TestSecretUseHasNoValueBearingField is the honest version of what the previous subtest claimed.
//
// The prior assertion marshaled a SecretUse and searched the JSON for "value" — but every field in this
// package carries `omitempty`, so a future `Value string` left UNSET would be omitted and the search
// would not fire. It only caught a value field LACKING `omitempty` — a guard that reads as protection
// and provides none.
//
// This asserts on the TYPE'S FIELD SET instead: a value-bearing field fails here the moment it is
// declared, whatever its tags and whether or not it is set.
func TestSecretUseHasNoValueBearingField(t *testing.T) {
	rt := reflect.TypeOf(spec.SecretUse{})
	allowed := map[string]bool{"Src": true, "Dst": true, "Optional": true}
	for i := 0; i < rt.NumField(); i++ {
		if name := rt.Field(i).Name; !allowed[name] {
			t.Fatalf("spec.SecretUse grew a field %q — a secret use may carry PATHS and `optional`, nothing else "+
				"(D-SEC-1: a value never becomes text)", name)
		}
	}
	for name := range allowed {
		if _, ok := rt.FieldByName(name); !ok {
			t.Fatalf("spec.SecretUse lost %q — the delivery contract needs it", name)
		}
	}
}

// TestOptionalReachesTheWire pins the one behavioural flag: a flag that cannot be expressed cannot be
// honoured.
func TestOptionalReachesTheWire(t *testing.T) {
	raw, err := json.Marshal(spec.SecretUse{Optional: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"optional":true`) {
		t.Fatalf("optional must reach the wire: %s", raw)
	}
}

// TestTheSecretContractIsDesigns is the port's identity check: schema/secret.cue is DESIGN.cue's
// declaration MOVED into the contract module, so the wire keys are DESIGN's own.
func TestTheSecretContractIsDesigns(t *testing.T) {
	raw, err := json.Marshal(spec.SecretUse{
		Src: "/run/charly/secrets/db_password",
		Dst: "/etc/app/db_password",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	for _, want := range []string{`"src"`, `"dst"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("the delivery pair is missing %s from %s", want, got)
		}
	}
}
