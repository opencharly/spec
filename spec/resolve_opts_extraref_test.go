package spec

import "testing"

// ExtraCandyRefStrings is the projection the plugin-word collectors use; it must return exactly
// the ref words IN ORDER and drop only the scope. A regression here silently mis-scopes or
// mis-names a plugin word, so it is pinned.
func TestExtraCandyRefStringsProjectsRefWordsInOrder(t *testing.T) {
	in := ScopedExtraCandyRefs(BoxScope("one-box"), "alpha", "@github.com/acme/plugin-beta:v1")
	got := ExtraCandyRefStrings(in)
	if len(got) != 2 || got[0] != "alpha" || got[1] != "@github.com/acme/plugin-beta:v1" {
		t.Fatalf("ExtraCandyRefStrings = %v, want [alpha @github.com/acme/plugin-beta:v1]", got)
	}
	if ExtraCandyRefStrings(nil) != nil {
		t.Fatalf("nil input must project to nil")
	}
	// The scope is dropped from the projection but preserved on the source entries.
	if in[0].Scope != "box=one-box" {
		t.Fatalf("source entry lost its scope: %+v", in[0])
	}
}

// The scope grammar has ONE owner: a caller and the collector must build the SAME label.
func TestScopeGrammarIsStable(t *testing.T) {
	if BoxScope("x") != "box=x" || LayerScope("y") != "layer=y" || KindLocalScope("z") != "kind:local=z" {
		t.Fatalf("scope grammar drifted: %q %q %q", BoxScope("x"), LayerScope("y"), KindLocalScope("z"))
	}
}

// ScopeIsBox is the SINGLE owner of the conflict-eligibility rule (R3): a box is the only
// composition unit two references genuinely share, so only a "box=" scope may conflict. Every
// other scope form the grammar produces — plus the empty "no scope" — is silent, and the
// predicate must agree with the constructors rather than spell the prefix itself
// (opencharly/charly#739).
func TestScopeIsBoxIsTheOnlyConflictEligibleScope(t *testing.T) {
	if !ScopeIsBox(BoxScope("any-box")) {
		t.Fatalf("a box scope must be conflict-eligible: %q", BoxScope("any-box"))
	}
	for _, silent := range []string{LayerScope("layer-x"), KindLocalScope("tpl"), "", "deploy=x", "box"} {
		if ScopeIsBox(silent) {
			t.Errorf("scope %q must NOT be conflict-eligible (only a box= scope)", silent)
		}
	}
}
