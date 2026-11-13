package spec

import "strings"

// plugin_tree_manifest.go — the ONE spelling of a plugin tree's `<leaf>.providers` manifest.
//
// A plugin tree is a directory of provider binaries in which a bare command/verb WORD resolves
// only through a manifest beside the binary: the loader maps a word to a file by reading
// `<leaf>.providers` (one `class:word` capability per line). A tree of binaries with no manifest
// therefore answers every invocation with charly's usage text — MEASURED 2026-11-13 on a disposable
// guest: the tree was delivered, `CHARLY_PLUGIN_DIR` pointed at it, and `charly firecracker --help`
// still printed the usage text until the manifest existed.
//
// TWO writers produce that layout — `sdk/deploykit`'s baked-image emitter (`EmitBakedPlugins`,
// into the image) and a `class:step` that delivers a tree at a venue — and they must produce the
// SAME BYTES, because the reader is one reader. This file is that one format; the SDK's emitter
// and any delivering step call it rather than carrying their own `strings.Join`.
const (
	// ManifestSuffix is the suffix a plugin manifest's filename carries: `<leaf><suffix>`.
	ManifestSuffix = ".providers"

	// ManifestMode is the mode every manifest is written with: readable, not executable. A
	// manifest is data the loader reads, never a program.
	ManifestMode = 0o644
)

// PluginManifest renders the manifest BYTES for a provider's declared words: one `class:word`
// capability per line, newline-terminated — the format `spec/exec`'s tree probe counts
// (`-name '*<suffix>'`) and the loader parses.
func PluginManifest(providers []string) []byte {
	if len(providers) == 0 {
		// A manifest with no words is not a manifest: the loader would read a binary nothing can
		// address, and `ValidatePluginTree` counts manifests, so an empty one would pass a
		// "is this a tree" probe while serving nothing. Callers must refuse before writing.
		return nil
	}
	return []byte(strings.Join(providers, "\n") + "\n")
}

// PluginManifestName is the manifest's FILENAME for a delivered binary: the binary's name plus
// ManifestSuffix. ONE derivation, so a writer cannot place the manifest where the loader will not
// look.
func PluginManifestName(binName string) string { return binName + ManifestSuffix }
