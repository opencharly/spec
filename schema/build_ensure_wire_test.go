package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// buildEnsureRequest compiles the shipped schema and unifies #BuildEnsureRequest
// with the given document, returning the unification/validation error (nil when
// the document satisfies the def).
//
// It mirrors deploy_from_test.go's helper: same concatenation the runtime and
// `charly task cue-gen` use, so this exercises the schema as it actually ships.
func buildEnsureRequest(t *testing.T, doc string) error {
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
	def := v.LookupPath(cue.ParsePath("#BuildEnsureRequest"))
	if !def.Exists() {
		t.Fatal("#BuildEnsureRequest is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(doc))
	if err := unified.Err(); err != nil {
		return err
	}
	return unified.Validate(cue.Concrete(false))
}

// POSITIVE CONTROL — it runs FIRST and is not optional. Without it, a loader that
// failed to find or compile the def would make the negative arm below pass
// vacuously (an absent def unifies with anything), which is the false-green this
// repository has been bitten by before.
func TestBuildEnsureRequestAcceptsRunEngine(t *testing.T) {
	if err := buildEnsureRequest(t, `{image: "some-box", run_engine: "podman"}`); err != nil {
		t.Fatalf("a legitimate build:ensure envelope was rejected: %v", err)
	}
	// The image is the only required field; the optional one is genuinely optional.
	if err := buildEnsureRequest(t, `{image: "some-box"}`); err != nil {
		t.Fatalf("an envelope carrying only the required image was rejected: %v", err)
	}
}

// The retired builder selector must be REJECTED by the closed def. Ensured
// presence is a property of the RUN engine's store, and the builder is BuildKit —
// not an engine — so there is no build-engine field to carry.
//
// This test FAILS without the cutover: while `build_engine` was still declared,
// this document unified cleanly and the negative arm below never fired.
func TestBuildEnsureRequestRejectsRetiredBuildEngine(t *testing.T) {
	if err := buildEnsureRequest(t, `{image: "some-box", build_engine: "podman"}`); err == nil {
		t.Fatal("a stale build_engine must be rejected by the closed #BuildEnsureRequest def")
	}
}
