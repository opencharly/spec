package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// deployFrom compiles the shipped schema and unifies #Deploy with the given
// from: literal, returning whether it satisfies the schema.
func deployFrom(t *testing.T, from string) error {
	t.Helper()
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#Deploy"))
	if !def.Exists() {
		t.Fatal("#Deploy is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString("from: " + from))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(false))
}

// The namespace-qualified from: form (ns.entity — a git-linked import ref) is
// the canonical reference-resolution surface's counterpart (loaderkit
// ResolveEntityRef, sdk #246): a bed in one repo may clone a golden from an
// imported namespace. This literal is the eval-omarchy X3 spike's ref.
func TestDeployFromAcceptsNamespaceQualifiedRef(t *testing.T) {
	if err := deployFrom(t, `"omarchy.check-charly-omarchy-vm:golden"`); err != nil {
		t.Fatalf("namespace-qualified from: ref rejected: %v", err)
	}
}

// The local NAME:TAG spelling stays legal (the loader splits the last colon
// into from + from_snapshot).
func TestDeployFromAcceptsLocalNameTag(t *testing.T) {
	if err := deployFrom(t, `"check-omarchy-eval-base-inst:golden"`); err != nil {
		t.Fatalf("local NAME:TAG from: ref rejected: %v", err)
	}
}

// A malformed ref (double dot, leading dot) must still be rejected.
func TestDeployFromRejectsMalformedRef(t *testing.T) {
	for _, bad := range []string{`"omarchy..check-charly-omarchy-vm"`, `".omarchy-vm"`, `"omarchy.check-charly-omarchy-vm:"`} {
		if err := deployFrom(t, bad); err == nil {
			t.Fatalf("malformed from: ref %s accepted", bad)
		}
	}
}
