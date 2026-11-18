// vocab_set_test.go — the ONE derivation of the #ResourceKind membership question.
//
// DESIGN.md S3 (line 869) made the contract the owner of the derivation: "the table is keyed by
// word and its rows carry the word's data … the word list is derived", and `@vocab` names which
// module owns the set. The word list IS derived here (`spec/vocab_gen.go`); before this accessor
// the MEMBERSHIP question was derived twice outside the contract — `loaderkit/parse.go:35`
// `sliceSet(spec.ResourceKinds)` and `charly/reserved_registry.go:46`
// `setFromSlice(spec.ResourceKinds)` — two byte-identical helpers in two repos over one
// contract-owned list.
package spec

import "testing"

// TestResourceKindSetAgreesWithTheVocabulary: the set is the vocabulary, not a copy of a
// remembered count. Asserted as a COMPUTED comparison against the generated list, so a word added
// to `#ResourceKind` (and regenerated) moves both sides and this test keeps meaning something.
func TestResourceKindSetAgreesWithTheVocabulary(t *testing.T) {
	set := ResourceKindSet()
	if len(set) != len(ResourceKinds) {
		t.Fatalf("set has %d words, the vocabulary declares %d", len(set), len(ResourceKinds))
	}
	for _, w := range ResourceKinds {
		if !set[w] {
			t.Fatalf("vocabulary word %q is absent from the set", w)
		}
	}
}

// TestResourceKindSetIsFreshPerCall: the caller owns its map. A set shared between two consumers —
// or handed to a caller that writes to it — is the shared-mutable-state shape this accessor must
// not create (D-STORE-1's lesson: a cached table handed out is not a derivation, it is a leak).
func TestResourceKindSetIsFreshPerCall(t *testing.T) {
	first := ResourceKindSet()
	if len(first) == 0 {
		t.Fatal("empty set: the vocabulary is never empty")
	}
	// Write through the caller's copy.
	for w := range first {
		delete(first, w)
		break
	}
	first["not-a-kind"] = true

	second := ResourceKindSet()
	if second["not-a-kind"] {
		t.Fatal("a caller's write leaked into a later caller's set: the accessor hands out one shared map")
	}
	if len(second) != len(ResourceKinds) {
		t.Fatalf("second call has %d words after a caller emptied its own copy; want the full vocabulary (%d)", len(second), len(ResourceKinds))
	}
}
