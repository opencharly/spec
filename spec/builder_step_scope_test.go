package spec

import "testing"

// TestBuilderStepScopeIsData pins that a builder's scope comes from its DATA
// (the plugin-declared package output), never its word (opencharly/sdk#608). Both directions are asserted, so a
// regression to `if s.Builder == "aur"` fails the second case and a regression to "always
// user" fails the first.
func TestBuilderStepScopeIsData(t *testing.T) {
	if got := (&BuilderStep{Builder: "npm", Package: &BuilderPackage{Format: "pac", Dir: "/tmp/aur-pkgs/c", Glob: "*.pkg.tar.zst"}}).Scope(); got != ScopeSystem {
		t.Errorf("a builder whose payload is OS packages has scope %v, want system", got)
	}
	if got := (&BuilderStep{Builder: "aur", BuilderDef: &BuilderDef{}}).Scope(); got != ScopeUser {
		t.Errorf("scope read the word `aur` instead of the data: got %v, want user for a builder with no package", got)
	}
}

// A distro's formats are its OS package formats only, so the primary format is simply the
// first one (no `secondary` to skip).
func TestPrimaryFormatIsTheFirstFormat(t *testing.T) {
	if got := primaryFormat(map[string]*Format{"rpm": {}, "pac": {}}); got != "pac" {
		t.Errorf("primaryFormat = %q, want the first sorted format pac", got)
	}
	if got := primaryFormat(nil); got != "" {
		t.Errorf("primaryFormat(nil) = %q, want empty", got)
	}
}
