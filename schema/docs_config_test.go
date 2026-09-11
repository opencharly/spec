package schema_test

// docs_config_test.go — the #DocsConfig closedness corpus: the docs: kind VALUE
// contract (schema/docs.cue) must accept the minimal + full authored configs and
// REJECT anything outside the closed struct (unknown fields, wrong types) —
// exactly the mirror of the marketplace:/skill:/hook: kind-value gates. The
// kind word recognition itself is NOT special-cased here (see
// spec/docs_config_test.go — docs routes through the generic node-form
// mechanism, no reserved-directive entry, no hand-maintained word list; R3).

import (
	"testing"
)

// minimalDocs is the smallest meaningful config: every field relying on its
// schema default (enabled + the compiled corpus paths), nothing elaborated.
const minimalDocs = `{
	sources: {
		compiled: {
			enabled:               true
			compiled_plugins_path: "charly/charly.yml"
			go_mod_path:           "charly/charly/go.mod"
		}
	}
}`

// fullDocs is the complete authored config: the release/extra repo lists, the
// marketplace layout, every projection toggle, the hand-authored tree, the
// landing source and all three gates.
const fullDocs = `{
	sources: {
		compiled: {
			enabled:               true
			compiled_plugins_path: "charly/charly.yml"
			go_mod_path:           "charly/charly/go.mod"
		}
		release_repos: ["plugin-review", "plugin-pipeline"]
		extra_repos:   ["plugin-gh"]
	}
	marketplace: {
		path: "marketplace"
	}
	projections: {
		recipes:   true
		cli:       true
		providers: true
		candy:     true
		box:       true
		plugin:    true
		landing:   true
	}
	output: {
		hand_authored: ["start", "concepts", "guides"]
	}
	landing: {
		readme: "README.md"
	}
	gates: {
		site_links:    true
		sidebar_links: true
		prune:         true
	}
}`

// TestDocsConfig_ValidatesMinimalConfig — the defaulted minimum must validate
// (every field present here is a documented default; the generator runs with
// these when a docs: node elaborates nothing else).
func TestDocsConfig_ValidatesMinimalConfig(t *testing.T) {
	if err := unifyDef(t, "#DocsConfig", minimalDocs); err != nil {
		t.Errorf("#DocsConfig rejects the minimal defaulted config:\n%v", err)
	}
}

// TestDocsConfig_ValidatesFullConfig — the complete authored surface.
func TestDocsConfig_ValidatesFullConfig(t *testing.T) {
	if err := unifyDef(t, "#DocsConfig", fullDocs); err != nil {
		t.Errorf("#DocsConfig rejects the full config:\n%v", err)
	}
}

// TestDocsConfig_RejectsUnknownFields — CLOSEDNESS: a typo'd or foreign key is
// a hard rejection, never silently ignored (the B12-provable half of the closed
// struct — the generator must never run on a config that says what it does not
// know).
func TestDocsConfig_RejectsUnknownFields(t *testing.T) {
	for _, tc := range []struct{ name, patch string }{
		{"top level", `{bogus: true}`},
		{"inside sources", `{sources: {bogus: true}}`},
		{"inside sources.compiled", `{sources: {compiled: {bogus: true}}}`},
		{"inside projections", `{projections: {bogus: true}}`},
		{"inside output", `{output: {bogus: ["x"]}}`},
		{"inside landing", `{landing: {bogus: "x"}}`},
		{"inside gates", `{gates: {bogus: true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDef(t, "#DocsConfig", tc.patch); err == nil {
				t.Error("#DocsConfig accepted an unknown field — the closed struct constrains nothing")
			}
		})
	}
}

// TestDocsConfig_RejectsWrongTypes — the field types stay constraining (a bool
// toggle taking a string, or a path taking a bool, must fail — never coerce).
func TestDocsConfig_RejectsWrongTypes(t *testing.T) {
	for _, tc := range []struct{ name, patch string }{
		{"enabled as a string", `{sources: {compiled: {enabled: "enabled"}}}`},
		{"compiled_plugins_path as a bool", `{sources: {compiled: {compiled_plugins_path: true}}}`},
		{"release_repos element empty", `{sources: {release_repos: [""]}}`},
		{"readme as a bool", `{landing: {readme: true}}`},
		{"prune as a list", `{gates: {prune: [true]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDef(t, "#DocsConfig", tc.patch); err == nil {
				t.Error("#DocsConfig accepted a wrong-typed field — the field constrains nothing")
			}
		})
	}
}
