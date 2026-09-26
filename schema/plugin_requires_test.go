package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// candyView unifies the shipped #CandyView with the given fragment, returning the
// unify/validate error (nil when the fragment is accepted).
func candyView(t *testing.T, fragment string) error {
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
	def := v.LookupPath(cue.ParsePath("#CandyView"))
	if !def.Exists() {
		t.Fatal("#CandyView is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(fragment))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(false))
}

// #CandyView carries the candy's OWN `plugin.requires:` list as `plugin_requires`, so
// the resolved view reaches a plugin's declared inter-plugin dependencies through the
// SAME path `plugin_source`/`plugin_providers` take. #CandyView is a CLOSED struct, so
// the field's ABSENCE (the pre-change schema) rejects this fragment outright — the test
// fails without the schema addition.
func TestCandyViewCarriesPluginRequires(t *testing.T) {
	if err := candyView(t, `plugin_requires: [{capability: "verb:enc"}]`); err != nil {
		t.Fatalf("plugin_requires rejected on #CandyView: %v", err)
	}
	// A full entry: capability identity + the peer's source + optional.
	if err := candyView(t, `plugin_requires: [{capability: "command:generate:box", source: "github.com/opencharly/plugin-build/candy/plugin-build", optional: true}]`); err != nil {
		t.Fatalf("full plugin_requires entry rejected: %v", err)
	}
}

// A malformed capability identity is rejected by the reused #PluginRequirement /
// #PluginCapability vocabulary — the projection does not open a second, looser shape.
func TestCandyViewRejectsMalformedPluginRequires(t *testing.T) {
	for _, bad := range []string{
		`plugin_requires: [{capability: "bogus:x"}]`,
		`plugin_requires: [{capability: "verb:"}]`,
		`plugin_requires: [{capability: "verb:enc", source: "not-a-github-ref"}]`,
	} {
		if err := candyView(t, bad); err == nil {
			t.Fatalf("malformed plugin_requires %s accepted", bad)
		}
	}
}
