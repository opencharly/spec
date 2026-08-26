package spec

import "testing"

// TestExternalBuilderPluginRef_StandaloneRef pins the Phase-4 ref shape: an externalized builder
// word resolves to the STANDALONE plugin-candy ref (github.com/opencharly/plugin-builder-<word>/
// candy/<name>), NOT the pre-cutover in-repo path — the connect fetch would hang on a deleted
// candy/<name> dir.
func TestExternalBuilderPluginRef_StandaloneRef(t *testing.T) {
	for word, want := range map[string]string{
		"pixi":  "@github.com/opencharly/plugin-builder-pixi/candy/plugin-builder-pixi",
		"cargo": "@github.com/opencharly/plugin-builder-cargo/candy/plugin-builder-cargo",
		"npm":   "@github.com/opencharly/plugin-builder-npm/candy/plugin-builder-npm",
		"aur":   "@github.com/opencharly/plugin-builder-aur/candy/plugin-builder-aur",
	} {
		got, ok := ExternalBuilderPluginRef(word)
		if !ok {
			t.Fatalf("%s: not recognized as externalized", word)
		}
		if got != want {
			t.Fatalf("%s: ref = %q, want %q", word, got, want)
		}
	}
	if _, ok := ExternalBuilderPluginRef("nonexistent"); ok {
		t.Fatal("unknown word should not resolve")
	}
}
