package spec_test

// provide_authoring_test.go — the authored `provide:` key (DESIGN.md:572) and the ONE rule that keeps
// its decomposition from becoming two-surface drift.
//
// The legacy maps (env_provide / mcp_provide / agent_provide) stay accepted only until the authored
// sites counted on spec#368 have migrated. What makes that a WINDOW rather than a permanent second
// surface is the schema's own refusal: a manifest that authors `provide:` AND a legacy map is a
// schema error. Migration may add the new key; nothing may hold both.
//
// The four arms below are that contract, in the order a reviewer would check it.

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

func validateCandy(t *testing.T, doc string) error {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	v := cuecontext.New().CompileString(src)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#Candy"))
	if !def.Exists() {
		t.Fatal("#Candy does not resolve in the shipped schema")
	}
	probe := v.Context().CompileString(doc)
	if probe.Err() != nil {
		return probe.Err()
	}
	return def.Unify(probe).Validate(cue.Concrete(true))
}

// TestProvideIsTheAuthoredSurface: `provide:` authored the way DESIGN.md:572 spells it validates.
func TestProvideIsTheAuthoredSurface(t *testing.T) {
	if err := validateCandy(t, `{description: "d", provide: {env: {FOO: {value: "1"}}}}`); err != nil {
		t.Fatalf("an authored `provide:` must validate: %v", err)
	}
}

// TestLegacyProvideMapIsStillAcceptedForTheWindow: the 170 unmigrated manifests keep loading.
func TestLegacyProvideMapIsStillAcceptedForTheWindow(t *testing.T) {
	if err := validateCandy(t, `{description: "d", env_provide: {FOO: "1"}}`); err != nil {
		t.Fatalf("a legacy provide map must still validate until spec#368 reaches zero: %v", err)
	}
}

// TestAuthoringBothSurfacesIsRefused is the guard: R2 enforced rather than commented.
func TestAuthoringBothSurfacesIsRefused(t *testing.T) {
	err := validateCandy(t, `{description: "d", provide: {env: {FOO: {value: "1"}}}, env_provide: {FOO: "1"}}`)
	if err == nil {
		t.Fatal("a candy authoring BOTH `provide:` and a legacy map must be a schema error — the duplication must not be able to grow")
	}
	if !strings.Contains(err.Error(), "never both") {
		t.Fatalf("the refusal must name the rule so the author knows what to delete: %v", err)
	}
}

// TestProvideRefusesATypeWithNoProvideRow: the §8 table still decides WHICH types may be provided.
// `secret` declares no provide row (spec#368), so authoring one is a schema error — the same
// refusal A-0's gatherer raises at runtime, here at the earliest possible moment.
func TestProvideRefusesATypeWithNoProvideRow(t *testing.T) {
	if err := validateCandy(t, `{description: "d", provide: {secret: {k: {}}}}`); err == nil {
		t.Fatal("`secret` has no provide row in the capability table, so authoring one must be refused")
	}
}
