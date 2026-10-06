package spec

// marketplace_config_test.go — the `marketplace` kind's `version` OPTIONALITY
// coverage for the #Marketplace contract (schema/marketplace.cue).
//
// WHY THIS TEST EXISTS. The org-wide cutover ("drop the retired config version:
// stamp") removed the `version:` stamp from every authored `marketplace:` entity,
// because the marketplace versions by commit SHA (marketplace/README.md: "No
// `version` fields anywhere"). The cutover did NOT update the schema, so
// `#Marketplace.version` stayed REQUIRED and the one repo carrying a
// `marketplace:` entity failed its own `charly box validate` gate with
// `#MarketplaceInput.version: incomplete value`.
//
// The two cases below are the whole contract, and case (a) is the one that FAILS
// on the pre-change schema — it is the reason this change exists:
//
//   (a) a marketplace body with NO `version` MUST validate (the new optionality);
//   (b) a legacy body carrying a CalVer `version` MUST still validate (back-compat).
//
// It compiles the SAME concatenation the runtime gate runs (schemaconcat over the
// embedded schema — R3, one concatenation contract), mirroring docs_config_test.go.

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

func TestMarketplaceVersionIsOptional(t *testing.T) {
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	ctx := cuecontext.New()
	compiled := ctx.CompileString(src)
	if compiled.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", compiled.Err())
	}
	gate := compiled.LookupPath(cue.ParsePath("#Marketplace"))
	if !gate.Exists() {
		t.Fatal("#Marketplace is not defined in the shipped schema")
	}

	// (a) THE NEW BEHAVIOUR — a version-less marketplace body must validate.
	// Concrete(true) is load-bearing: Concrete(false) PERMITS incomplete values, so
	// a missing required field would pass and this would be a mere compile check.
	// With Concrete(true) the pre-change schema (version required) FAILS here, which
	// is what makes this test discriminating.
	noVersion := compiled.Context().CompileString(`{
		name: "charly-plugins"
		families: {
			core: {category: "commands"}
		}
	}`)
	unified := gate.Unify(noVersion)
	if unified.Err() != nil {
		t.Fatalf("#Marketplace rejected a version-less marketplace body (the cutover's own shape):\n%v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("#Marketplace validation failed for the version-less body:\n%v", err)
	}
	t.Log("version-less marketplace body: ACCEPTED (the stamp is optional)")

	// (b) BACK-COMPAT — a legacy body carrying a CalVer version must still validate,
	// so an on-disk config written before the cutover keeps loading.
	withVersion := compiled.Context().CompileString(`{
		name: "charly-plugins"
		version: "2026.272.0616"
		families: {
			core: {category: "commands"}
		}
	}`)
	unifiedLegacy := gate.Unify(withVersion)
	if unifiedLegacy.Err() != nil {
		t.Fatalf("#Marketplace rejected a legacy version-bearing marketplace body:\n%v", unifiedLegacy.Err())
	}
	if err := unifiedLegacy.Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("#Marketplace validation failed for the version-bearing body:\n%v", err)
	}
	t.Log("legacy version-bearing marketplace body: ACCEPTED (back-compat held)")

	// (c) The pattern still rejects a MALFORMED version, so optionality did not
	// become "anything goes" — the CalVer regex survives.
	badVersion := compiled.Context().CompileString(`{
		name: "charly-plugins"
		version: "not-a-calver"
		families: {core: {category: "commands"}}
	}`)
	if gate.Unify(badVersion).Validate(cue.Concrete(true)) == nil {
		t.Fatal("#Marketplace ACCEPTED a malformed version — the CalVer pattern was lost")
	}
	t.Log("malformed version: REJECTED (the CalVer pattern survives)")
}
