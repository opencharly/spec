// vocab_set.go — the ONE derivation of the #ResourceKind membership question.
//
// DESIGN.md S3 (line 869) makes the contract the owner of every vocabulary derivation: the table
// is keyed by word and its rows carry the word's data, "the word list is derived" from it, and
// `@vocab` names the module that owns the set. `ResourceKinds` (`spec/vocab_gen.go`) is that
// derived list, and it is generated — never hand-edited.
//
// Membership — "is this word a resource kind?" — was derived OUTSIDE the contract, twice, by two
// byte-identical helpers over this one list: `sliceSet(spec.ResourceKinds)` in
// `sdk/loaderkit/parse.go` and `setFromSlice(spec.ResourceKinds)` in
// `charly/reserved_registry.go`. One input, two private copies, two repos: the duplication R2
// exists to end, and the reason it belongs HERE is that the list belongs here.
//
// DESTINATION, not yet reached. DESIGN.md C3 (line 1074) prescribes a vocabulary word as "a typed
// word with `Valid()`", and a membership set is not that. The generated artifact is today a bare
// []string, and the typed-word form exists nowhere in this module yet (`grep -n ') Valid() bool'`
// over `spec/*.go` finds no hand-written or generated method). When the generator emits
// `type ResourceKind string` with `Valid()`, membership becomes `ResourceKind(w).Valid()` and this
// function leaves with its last caller. Until then it is a HELPER over the generated list, not a
// new shape: it declares no type and no vocabulary the schema does not already own.
package spec

// ResourceKindSet returns the membership set of the #ResourceKind vocabulary: every word of
// ResourceKinds, keyed true.
//
// A FRESH map on every call, deliberately. A set cached and handed to two consumers is a shared
// mutable table: a caller that writes to its copy would silently change another caller's
// classification (the shape D-STORE-1 was about). The vocabulary is a handful of words, so the
// derivation costs nothing beside the CUE unification that follows it.
func ResourceKindSet() map[string]bool {
	set := make(map[string]bool, len(ResourceKinds))
	for _, w := range ResourceKinds {
		set[w] = true
	}
	return set
}
