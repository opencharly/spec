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

// TestKindGateCarriesTheAuthoredAndWireShapesApart pins opencharly/opencharly#1048: a kind has TWO
// shapes — the authored body the host validates and fills, and the wire value a plugin's InputDef
// names (often a union that also admits the substrate arm). Carrying one name for both made the
// fill resolve the union for an authored body and refuse fixtures for a field the OTHER arm
// requires. The two travel separately, and an empty WireDef means they are the same def.
func TestKindGateCarriesTheAuthoredAndWireShapesApart(t *testing.T) {
	in := Threaded{
		KindGates: map[string]KindGate{
			"vm":  {Def: "#Vm", WireDef: "#VmValue", Source: "package schema\n#Vm: string\n#VmValue: (#Vm | string)"},
			"pod": {Def: "#Pod", Source: "package schema\n#Pod: string"},
		},
	}
	out, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Threaded
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	vm := back.KindGates["vm"]
	if vm.Def != "#Vm" || vm.WireDef != "#VmValue" {
		t.Fatalf("the two shapes must survive the wire apart, got Def=%q WireDef=%q", vm.Def, vm.WireDef)
	}
	if pod := back.KindGates["pod"]; pod.Def != "#Pod" || pod.WireDef != "" {
		t.Fatalf("a kind whose shapes match carries no WireDef, got Def=%q WireDef=%q", pod.Def, pod.WireDef)
	}
}
