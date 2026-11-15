package spec

import (
	"encoding/json"
	"testing"
)

// TestThreadedKindGatesAreClauseDData pins the gate carrier: a kind's declared def path and the
// served CUE source travel as PLAIN DATA on spec.Threaded, so a receiving process can compile the
// source locally without a registry query. It fails to compile against the pre-cutover Threaded
// (no KindGates field) and, at run time, if the pair stops surviving the wire.
func TestThreadedKindGatesAreClauseDData(t *testing.T) {
	in := Threaded{
		KindGates: map[string]KindGate{
			"candy": {Def: "#CandyValue", Source: "package schema\n#CandyValue: string"},
		},
	}
	raw, err := json.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Threaded
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	g, ok := out.KindGates["candy"]
	if !ok {
		t.Fatal("the gate did not survive the wire")
	}
	if g.Def != "#CandyValue" {
		t.Errorf("Def = %q, want #CandyValue", g.Def)
	}
	if g.Source == "" {
		t.Error("Source was dropped — the receiving side could not compile the declaration")
	}
}

// TestThreadedKindGatesAbsentIsInert pins the additive property: a Threaded that never fills the
// map behaves exactly as before, so the field is safe for every existing consumer.
func TestThreadedKindGatesAbsentIsInert(t *testing.T) {
	raw, err := json.Marshal(&Threaded{Kinds: map[string]bool{"pod": true}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Threaded
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.KindGates != nil {
		t.Errorf("KindGates = %v, want nil for a consumer that declares none", out.KindGates)
	}
	if !out.Kinds["pod"] {
		t.Error("the existing Kinds snapshot was lost")
	}
}
