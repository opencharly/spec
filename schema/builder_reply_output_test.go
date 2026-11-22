package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// builder_reply_output_test.go — DESIGN §4.4 says there is NO version negotiation: a plugin built
// against a different protocol shape "fails at its first message, naming the field" (D-PROTO-6).
// The builder resolve reply makes that true by REQUIRING its `output` discriminator, so a plugin
// that predates it cannot produce a valid reply at all (opencharly/sdk#615: the pre-unification
// plugin shipped a payload and no package, and nothing failed).
//
// Every case fails on the pre-change schema.

// The discriminator is required: the exact reply an old builder plugin sends — a stage and payload
// copies, no output — is invalid, and the host rejects it naming `output`.
func TestBuilderResolveReplyRequiresItsOutputDiscriminator(t *testing.T) {
	// A REQUIRED field is enforced on a concrete value — the validation a received message gets.
	oldStyle := `{stage: "FROM x AS y", copy_artifacts: ["COPY --from=y /out /out"]}`
	if err := unifyConcrete(t, "#BuilderResolveReply", oldStyle); err == nil {
		t.Fatal("#BuilderResolveReply accepted a reply with no `output`: an outdated plugin would pass validation again")
	}
}

// A package-producing builder declares it, with the package it produced.
func TestBuilderResolveReplyAcceptsAPackageOutput(t *testing.T) {
	doc := `{
		output: "package"
		stage: "FROM arch-builder AS c-aur-build"
		copy_artifacts: ["COPY --from=c-aur-build /tmp/aur-pkgs/ /tmp/aur-pkgs/"]
		package: {format: "pac", dir: "/tmp/aur-pkgs/c", glob: "*.pkg.tar.zst"}
	}`
	if err := unify(t, "#BuilderResolveReply", doc); err != nil {
		t.Fatalf("#BuilderResolveReply rejects a well-formed package output: %v", err)
	}
}

// A file-copying builder (npm/pixi/mise) declares its shape too, and needs no package.
func TestBuilderResolveReplyAcceptsAFileOutput(t *testing.T) {
	doc := `{output: "files", stage: "FROM node:20 AS c-npm-build", copy_artifacts: ["COPY --from=c-npm-build /usr/local/lib/node_modules /usr/local/lib/node_modules"]}`
	if err := unify(t, "#BuilderResolveReply", doc); err != nil {
		t.Fatalf("#BuilderResolveReply rejects a well-formed files output: %v", err)
	}
}

// An inline builder declares that shape.
func TestBuilderResolveReplyAcceptsAnInlineOutput(t *testing.T) {
	if err := unify(t, "#BuilderResolveReply", `{output: "inline", inline_fragment: "RUN cargo build"}`); err != nil {
		t.Fatalf("#BuilderResolveReply rejects a well-formed inline output: %v", err)
	}
}

// The vocabulary is closed: an unknown discriminator is a schema error, not a free-form string.
func TestBuilderResolveReplyRejectsAnUnknownOutput(t *testing.T) {
	if err := unify(t, "#BuilderResolveReply", `{output: "binary", copy_binary: "x"}`); err == nil {
		t.Fatal("#BuilderResolveReply accepted an unknown `output` value")
	}
}

// unifyConcrete is unify() for a value that must be COMPLETE: a required field is enforced only on
// a concrete value, which is exactly how a received message is judged (D-PROTO-6).
func unifyConcrete(t *testing.T, def, doc string) error {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	v := cuecontext.New().CompileString(src)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	d := v.LookupPath(cue.ParsePath(def))
	if !d.Exists() {
		t.Fatalf("%s is not defined in the shipped schema", def)
	}
	u := d.Unify(v.Context().CompileString(doc))
	if u.Err() != nil {
		return u.Err()
	}
	return u.Validate(cue.Concrete(true))
}
