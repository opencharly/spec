package spec

import (
	"encoding/json"
	"testing"
)

// TestSetFolds_NilAllocAndRoundTrip locks the SHARED set-body helpers (parser consolidation
// F2.4): SetBoxInto/SetCandyInto allocate a nil map in place AND round-trip through the typed
// decode — the exact fold the box⊻layer factory dispatch writes acc.Box/acc.Candy with, and the
// fold Config.SetBox / UnifiedFile.SetBox / UnifiedFile.SetCandy now share (R3 — ONE set-body
// fold, never three inline copies).
func TestSetFolds_NilAllocAndRoundTrip(t *testing.T) {
	// Box: nil map is allocated by the fold.
	var m BoxMap
	m = SetBoxInto(m, "img", BoxConfig{Description: "x"})
	if m == nil || m["img"] == nil {
		t.Fatal("SetBoxInto must allocate a nil map and store the encoded body")
	}
	b, ok := DecodeBox(m["img"])
	if !ok || b.Description != "x" {
		t.Fatalf("SetBoxInto round-trip decode = %+v, want description x", b)
	}

	// Candy: nil map is allocated by the fold.
	var c map[string]json.RawMessage
	c = SetCandyInto(c, "layer", &InlineCandy{CandyYAML: CandyYAML{Description: "y"}})
	if c == nil || c["layer"] == nil {
		t.Fatal("SetCandyInto must allocate a nil map and store the encoded body")
	}
	il, ok := DecodeInlineCandy(c["layer"])
	if !ok || il.Description != "y" {
		t.Fatalf("SetCandyInto round-trip decode = %+v, want description y", il)
	}

	// The typed setters route through the same folds.
	uf := &UnifiedFile{}
	uf.SetBox("img2", BoxConfig{Description: "z"})
	if _, ok := uf.Box["img2"]; !ok {
		t.Fatal("UnifiedFile.SetBox must store via the shared fold")
	}
	uf.SetCandy("layer2", &InlineCandy{CandyYAML: CandyYAML{Description: "w"}})
	if _, ok := uf.Candy["layer2"]; !ok {
		t.Fatal("UnifiedFile.SetCandy must store via the shared fold")
	}
}
