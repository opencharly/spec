package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// pluginValue compiles the shipped schema, looks up #Plugin, and returns whether
// the given authored plugin body unifies + validates against it. A non-nil error
// means the body is rejected (a schema conflict or a concrete-ness failure).
func pluginValue(t *testing.T, body string) error {
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
	pl := v.LookupPath(cue.ParsePath("#Plugin"))
	if !pl.Exists() {
		t.Fatal("#Plugin is not defined in the shipped schema")
	}
	unified := pl.Unify(ctx.CompileString(body))
	if unified.Err() != nil {
		return unified.Err()
	}
	// Concrete(true): the required fields (providers, source) must be PRESENT, not
	// merely consistent. Concrete(false) tolerates an incomplete value, so a body
	// missing a required field would pass.
	return unified.Validate(cue.Concrete(true))
}

// #Plugin.source is now a REQUIRED github.com module ref. The retired `builtin`
// sentinel must be rejected: compiled-in vs out-of-process is the charly.yml
// `compiled_plugins:` selection, not a manifest form. On the pre-change schema
// `source: builtin` was the DEFAULT and validated, so this case fails without the
// change.
func TestPluginSourceIsRequiredGithubRef(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"github ref accepted", `{providers: ["verb:x"], source: "github.com/opencharly/plugin-x/candy/plugin-x"}`, true},
		{"the retired builtin sentinel is rejected", `{providers: ["verb:x"], source: "builtin"}`, false},
		{"a missing source is rejected", `{providers: ["verb:x"]}`, false},
		{"a bare name (not github.com/...) is rejected", `{providers: ["verb:x"], source: "plugin-x"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := pluginValue(t, tc.body)
			if tc.ok && err != nil {
				t.Errorf("#Plugin rejected a valid body: %v", err)
			}
			if !tc.ok && err == nil {
				t.Errorf("#Plugin accepted a body it must reject")
			}
		})
	}
}

// #Plugin.requires declares an inter-plugin dependency. On the pre-change schema
// `requires` did not exist and #Plugin is CLOSED, so any body carrying it failed
// unification — these accepted cases fail without the change.
func TestPluginRequiresDeclared(t *testing.T) {
	const src = `source: "github.com/opencharly/plugin-x/candy/plugin-x"`
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"a bare capability requirement", `{providers: ["verb:x"], ` + src + `, requires: [{capability: "verb:enc"}]}`, true},
		{"a requirement with a source ref + optional", `{providers: ["verb:x"], ` + src + `, requires: [{capability: "verb:enc", source: "github.com/opencharly/plugin-enc/candy/plugin-enc", optional: true}]}`, true},
		{"no requires is fine", `{providers: ["verb:x"], ` + src + `}`, true},
		{"an unknown class is rejected", `{providers: ["verb:x"], ` + src + `, requires: [{capability: "bogus:enc"}]}`, false},
		{"a missing capability is rejected", `{providers: ["verb:x"], ` + src + `, requires: [{optional: true}]}`, false},
		{"an unknown requires field is rejected (CLOSED)", `{providers: ["verb:x"], ` + src + `, requires: [{capability: "verb:enc", typo: 1}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := pluginValue(t, tc.body)
			if tc.ok && err != nil {
				t.Errorf("#Plugin rejected a valid requires body: %v", err)
			}
			if !tc.ok && err == nil {
				t.Errorf("#Plugin accepted a requires body it must reject")
			}
		})
	}
}
