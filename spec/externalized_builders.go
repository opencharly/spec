package spec

// externalized_builders.go — the D-FACT of which detection-builder words are served by an EXTERNAL
// out-of-process plugin (no in-proc BuilderProvider). #55 import-purity: relocated from sdk/buildkit
// DOWN to spec (the wire/value leaf) so charly core reads the ONE source over its spec+proto-only
// import surface; sdk/buildkit keeps a thin var-forwarder for its plugin callers.

// ExternalizedBuilders is THE single source of truth for which builder words are served by an EXTERNAL
// out-of-process plugin. A word here resolves through the registry to a *grpcProvider connected at
// plugin-load time.
// mise: the builder:mise plugin (opencharly/plugin-mise) — externalized like the four
// detection builders; the embedded builder: mise: vocabulary in charly selects it by
// detect_file (mise.toml/.tool-versions).
var ExternalizedBuilders = map[string]bool{
	"cargo": true,
	"npm":   true,
	"pixi":  true,
	"aur":   true,
	"mise":  true,
}

// externalBuilderPlugins maps each externalized builder word to the STANDALONE plugin-candy ref
// that serves it (the candy de-submodule cutover, Phase 4: the plugin candies live in their own
// repos, github.com/opencharly/plugin-builder-<word>, at candy/plugin-builder-<word>/ inside).
// The SECOND half of the same D-fact, keyed identically to ExternalizedBuilders above (K-wave 2
// cone R1: relocated from charly/provider_builder_external.go, which held a byte-identical word
// list — an R3 duplicate of the map right above it).
var externalBuilderPlugins = map[string]string{
	"cargo": "plugin-builder-cargo/candy/plugin-builder-cargo",
	"npm":   "plugin-builder-npm/candy/plugin-builder-npm",
	"pixi":  "plugin-builder-pixi/candy/plugin-builder-pixi",
	"aur":   "plugin-builder-aur/candy/plugin-builder-aur",
	"mise":  "plugin-mise/candy/plugin-mise",
}

// ExternalBuilderPluginRef returns the canonical @github ref to the candy serving an externalized
// builder word, and whether the word is a first-party externalized builder. It feeds the ONE
// on-demand connect path — ops.InvokeProviderOpts.ExtraRef, which the host resolves through
// connectPluginByWordRef's Pass-2 canonical-ref fetch — so a box/<distro> submodule build that
// triggers a builder by DETECTION but vendors the plugin candy nowhere still connects it. Living in
// spec (beside the word maps it reads) lets BOTH charly core and the plugin-side render reach it
// over a spec-only import surface.
func ExternalBuilderPluginRef(word string) (string, bool) {
	ref, ok := externalBuilderPlugins[word]
	if !ok {
		return "", false
	}
	// The standalone plugin-candy ref: @github.com/opencharly/plugin-builder-<word>/candy/<name>
	// (the post-cutover layout — the connect path resolves it through the standalone fetch).
	return "@github.com/opencharly/" + ref, true
}
