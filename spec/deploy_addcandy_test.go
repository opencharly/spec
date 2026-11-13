package spec

import "testing"

// TestScopedAddCandyRefs pins the collection contract every consumer shares (opencharly/spec#309):
// the node's own refs then its in-substrate descendants', in authored order, all under ONE scope,
// with a deploy-level sibling excluded because it is its own composition.
func TestScopedAddCandyRefs(t *testing.T) {
	nested := &Deploy{AddCandy: []CandyRef{"@github.com/opencharly/layer-charly:v2026.292.2352"}}
	alongside := &Deploy{AddCandy: []CandyRef{"@github.com/opencharly/layer-ripgrep:v2026.235.1653"}}
	node := &Deploy{
		AddCandy: []CandyRef{"@github.com/opencharly/layer-direnv:v2026.287.2155", "local-candy"},
		Member: []Member{
			{Name: "host", Position: PositionInSubstrate, Node: nested},
			{Name: "sibling", Position: PositionDeployLevel, Node: alongside},
		},
	}

	scope := BoxScope("check-arch-vm")
	got := node.ScopedAddCandyRefs(scope)
	want := []ExtraCandyRef{
		{Ref: "@github.com/opencharly/layer-direnv:v2026.287.2155", Scope: scope},
		{Ref: "local-candy", Scope: scope},
		{Ref: "@github.com/opencharly/layer-charly:v2026.292.2352", Scope: scope},
	}
	if len(got) != len(want) {
		t.Fatalf("collected %d ref(s), want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ref[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// A node with nothing to contribute yields nothing, and a nil receiver must not panic: both are
	// reached on real trees (a deploy with no add_candy:; a member entry with no node body).
	if refs := (&Deploy{}).ScopedAddCandyRefs(scope); len(refs) != 0 {
		t.Fatalf("a node with no add_candy must collect nothing, got %+v", refs)
	}
	var nilNode *Deploy
	if refs := nilNode.ScopedAddCandyRefs(scope); refs != nil {
		t.Fatalf("a nil node must collect nothing, got %+v", refs)
	}
}
