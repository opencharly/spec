package schema_test

// vm_vocab_reexports_test.go — the test round 1 of opencharly/spec#304 unit 8's contract leg
// showed missing: the eight PUBLIC vocabulary re-exports must resolve in the shipped schema and
// must EQUAL their private derivations, so a misspelled public name or an alias pointing at the
// wrong list fails HERE rather than in the plugin that cites it.
//
// The S3 tests do not cover this surface: they exercise the private tables and their `_<x>Word`
// derivations, which are unchanged by the re-export. Neither does TestGenReproducible, which only
// asserts the committed Go matches the generated Go.

import (
	"testing"

	"cuelang.org/go/cue"
)

func TestVmVocabularyReExports(t *testing.T) {
	v := compileShipped(t)
	// The eight public names a sibling plugin cites, each with ONE word only its own list
	// carries: a wrong pointer (an alias aimed at a sibling's list) resolves and decodes
	// cleanly, so membership is what proves the pointer, not a non-empty decode.
	expect := map[string]string{
		"#VmBackendNameWord": "libvirt",
		"#VmBackendWord":     "auto",
		"#VmMachineWord":     "q35",
		"#VmCloudInitWord":   "enabled",
		"#VmSmbiosWord":      "disabled",
		"#VmStrategyWord":    "scp",
		"#VmFirmwareWord":    "uefi-secure",
		"#VmKindWord":        "container_disk",
	}
	for name, mustContain := range expect {
		def := v.LookupPath(cue.ParsePath(name))
		if !def.Exists() {
			t.Fatalf("%s does not resolve in the shipped schema — a sibling plugin cites it, so it must", name)
		}
		var words []string
		if err := def.Decode(&words); err != nil {
			t.Fatalf("%s does not decode to a word list: %v", name, err)
		}
		found := false
		for _, w := range words {
			if w == mustContain {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s = %v — its own list must contain %q, so the alias points at the RIGHT table", name, words, mustContain)
		}
	}
}
