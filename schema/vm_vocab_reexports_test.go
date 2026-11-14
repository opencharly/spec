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
	// The eight public names a sibling plugin cites. Each must RESOLVE and decode to a
	// non-empty string list: a removed or misspelled name fails the lookup, and an alias
	// pointing at a non-list fails the decode. (The private derivations cannot be looked up
	// by path — hidden labels are not addressable — so this pins the surface the sibling
	// actually uses rather than comparing to an unaddressable value.)
	public := []string{
		"#VmBackendNameWord",
		"#VmBackendWord",
		"#VmMachineWord",
		"#VmCloudInitWord",
		"#VmSmbiosWord",
		"#VmStrategyWord",
		"#VmFirmwareWord",
		"#VmKindWord",
	}
	for _, name := range public {
		def := v.LookupPath(cue.ParsePath(name))
		if !def.Exists() {
			t.Fatalf("%s does not resolve in the shipped schema — a sibling plugin cites it, so it must", name)
		}
		var words []string
		if err := def.Decode(&words); err != nil {
			t.Fatalf("%s does not decode to a word list: %v", name, err)
		}
		if len(words) < 2 {
			t.Fatalf("%s = %v — a vocabulary list carries at least two rows", name, words)
		}
	}
}
