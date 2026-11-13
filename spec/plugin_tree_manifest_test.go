package spec

// plugin_tree_manifest_test.go — the ONE plugin-tree manifest format (opencharly/charly#1131).
//
// The property under test is the one two writers depend on: the bytes and the filename are exactly
// what a plugin tree's readers expect, so a delivered tree and a baked image are the same layout.

import (
	"bytes"
	"testing"
)

func TestPluginManifestIsOneWordPerLineAndNewlineTerminated(t *testing.T) {
	got := PluginManifest([]string{"verb:firecracker", "command:firecracker"})
	want := []byte("verb:firecracker\ncommand:firecracker\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("PluginManifest = %q, want %q", got, want)
	}

	// A single word still ends with a newline: the loader splits on lines, and a last line with no
	// terminator is the shape that reads as a truncated manifest.
	if got := PluginManifest([]string{"command:mcp"}); !bytes.Equal(got, []byte("command:mcp\n")) {
		t.Fatalf("single-word manifest = %q, want %q", got, "command:mcp\n")
	}

	// NO words is NOT a manifest. A zero-word one would satisfy a "does this tree carry a manifest"
	// count while addressing nothing, which is the vacuous-tree class this format exists to prevent.
	if got := PluginManifest(nil); got != nil {
		t.Fatalf("PluginManifest(nil) = %q, want nil (a manifest with no words is not a manifest)", got)
	}
	if got := PluginManifest([]string{}); got != nil {
		t.Fatalf("PluginManifest(empty) = %q, want nil", got)
	}
}

func TestPluginManifestNameIsTheBinaryNamePlusTheSuffix(t *testing.T) {
	if got := PluginManifestName("plugin-firecracker"); got != "plugin-firecracker.providers" {
		t.Fatalf("PluginManifestName = %q, want plugin-firecracker.providers", got)
	}
	if ManifestSuffix != ".providers" {
		t.Fatalf("ManifestSuffix moved off the published suffix: %q", ManifestSuffix)
	}
	if ManifestMode != 0o644 {
		t.Fatalf("ManifestMode = %#o, want 0o644 (data, not a program)", ManifestMode)
	}
}
