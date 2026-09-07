package spec

import (
	"testing"
)

// The config: verb discriminates like every install verb: exactly one verb
// per op, resolved by Kind()/VerbsSet().
func TestOpConfigVerbKind(t *testing.T) {
	op := Op{Config: "/etc/crabbox.yaml", Content: "provider: local-container\n"}
	kind, err := op.Kind()
	if err != nil {
		t.Fatalf("config op Kind(): %v", err)
	}
	if kind != "config" {
		t.Fatalf("op.Kind() = %q, want config", kind)
	}
	set := op.VerbsSet()
	if len(set) != 1 || set[0] != "config" {
		t.Fatalf("op.VerbsSet() = %v, want [config]", set)
	}
}

// OpVerbs (CUE-derived #OpVerb) must list config — Kind()'s error message and
// the charly reserved-word registry bijection gate against it.
func TestOpVerbsIncludesConfig(t *testing.T) {
	for _, v := range OpVerbs {
		if v == "config" {
			return
		}
	}
	t.Fatal("OpVerbs does not list config")
}

// StringFields drives the in-place ${VAR} expansion (ExpandVars in charly).
// Content rides it ONLY for config ops: a write: body stays verbatim bytes
// (the documented write contract), while a config: body is substituted — the
// whole point of the verb.
func TestStringFieldsConfigExpandsContentOnlyForConfig(t *testing.T) {
	writeOp := Op{Write: "/etc/x", Content: "keep ${UNRESOLVED_BY_DESIGN}"}
	for _, f := range writeOp.StringFields() {
		if f == &writeOp.Content {
			t.Fatal("write content must stay out of StringFields (verbatim)")
		}
	}
	cfgOp := Op{Config: "/etc/crabbox.yaml", Content: "provider: ${CBX_CFG_PROVIDER}\n"}
	found := false
	for _, f := range cfgOp.StringFields() {
		if f == &cfgOp.Content {
			found = true
		}
	}
	if !found {
		t.Fatal("config content must be in StringFields (generate-time substitution)")
	}
	pathFound := false
	for _, f := range cfgOp.StringFields() {
		if f == &cfgOp.Config {
			pathFound = true
		}
	}
	if !pathFound {
		t.Fatal("config path field must be in StringFields")
	}
}
