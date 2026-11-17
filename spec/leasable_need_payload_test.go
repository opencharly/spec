package spec

// leasable_need_payload_test.go — the R7 half of the second finding on opencharly/spec#360.
//
// The map branch (spec#271) turns a pattern-keyed collection into a real map, which SURFACES
// whatever the element type really is. For the four LEASABLE capability types (cpu, memory,
// storage, mount) the element used to evaluate to BOTTOM -- `exclusive: field not allowed` --
// because `#Need`/`#Use` merge the lease fields into a CLOSED need shape, which is legal only
// with the postfix `...` DESIGN writes at the embedding site (refused by the pinned CUE v0.16.1
// without @experiment(explicitopen)). The generator could then spell the element only
// `map[string]any`, so a CPU need carried its KEY and nothing about what it asked for.
//
// schema/capability.cue now declares the lease field on the three leasable shapes, which is the
// CUE-v0.16.1 spelling of the same net shape. These assertions read the GENERATED types, so they
// fail if that schema ever regresses to a bottom element: `any` is not a struct, so the payload
// checks below cannot pass by accident.

import (
	"reflect"
	"testing"
)

// mapElement returns the element type of the named struct field, and fails unless that field is
// a map at all.
func mapElement(t *testing.T, owner reflect.Type, field string) reflect.Type {
	t.Helper()
	f, ok := owner.FieldByName(field)
	if !ok {
		t.Fatalf("%s has no field %s", owner.Name(), field)
	}
	if f.Type.Kind() != reflect.Map {
		t.Fatalf("%s.%s is %s, want a map — a pattern-keyed collection is not a struct", owner.Name(), field, f.Type)
	}
	if f.Type.Key().Kind() != reflect.String {
		t.Fatalf("%s.%s is keyed by %s, want string — Go cannot express a closed set as a key type", owner.Name(), field, f.Type.Key())
	}
	return f.Type.Elem()
}

// requireField fails unless the struct type carries the named field with the wanted Go type.
func requireField(t *testing.T, elem reflect.Type, field, want string) {
	t.Helper()
	if elem.Kind() != reflect.Struct {
		t.Fatalf("the map's element is %s, not a struct — the payload is lost (map[string]any is the same defect as struct{})", elem)
	}
	f, ok := elem.FieldByName(field)
	if !ok {
		t.Fatalf("the element type carries no %s — the payload is lost", field)
	}
	if got := f.Type.String(); got != want {
		t.Fatalf("the element's %s is %s, want %s", field, got, want)
	}
}

// TestLeasableNeedElementCarriesItsPayload is the assertion the four leasable types never had:
// a CPU need says HOW MANY, and a memory need says HOW MUCH, not merely that they are needed.
// TestSizeIsAnAliasAndCountIsNot pins the fact the payload table depends on: #Size emits an
// ALIAS so its field renders as "string", while #Count emits a DEFINED type so its field
// renders as "spec.Count". If a future schema change turns #Size into a defined type, that
// table's "string" expectation becomes wrong and this test names why.
func TestSizeIsAnAliasAndCountIsNot(t *testing.T) {
	size := reflect.TypeOf(Size(""))
	if size.Kind() != reflect.String || size.Name() != "string" {
		t.Fatalf("Size is no longer an ALIAS (Kind=%v Name=%q) -- the payload table's "+
			"`\"string\"` expectation for .Size must be updated to %q", size.Kind(), size.Name(), "spec."+size.Name())
	}
	count := reflect.TypeOf(Count(0))
	if count.Name() != "Count" {
		t.Fatalf("Count is no longer a DEFINED type (Name=%q) -- the payload table's "+
			"`\"spec.Count\"` expectation must be updated", count.Name())
	}
}

func TestLeasableNeedElementCarriesItsPayload(t *testing.T) {
	for _, c := range []struct {
		owner reflect.Type
		field string
		// want is the payload field the element must carry, with its Go type.
		wantField, wantType string
	}{
		{reflect.TypeOf(CandyNeed{}), "Cpu", "Count", "spec.Count"},
		// "string" rather than "spec.Size" is CORRECT and deliberate: #Size is declared
		// `@go(-)` and emits `type Size = string` (an ALIAS), while #Count is a real
		// `type Count int64`. reflect.Type.String() on an alias field renders the
		// underlying type, so a reader who sees "string" beside "spec.Count" must not
		// "fix" this to "spec.Size" -- that would fail. Asserted below, not assumed.
		{reflect.TypeOf(CandyNeed{}), "Memory", "Size", "string"},
		{reflect.TypeOf(CandyNeed{}), "Storage", "Size", "string"},
		{reflect.TypeOf(CandyNeed{}), "Mount", "Host", "spec.AbsPath"},
		{reflect.TypeOf(CandyUse{}), "Cpu", "Count", "spec.Count"},
		{reflect.TypeOf(CandyUse{}), "Memory", "Size", "string"},
		{reflect.TypeOf(CandyUse{}), "Storage", "Size", "string"},
		{reflect.TypeOf(CandyUse{}), "Mount", "Host", "spec.AbsPath"},
		{reflect.TypeOf(Need{}), "Cpu", "Count", "spec.Count"},
		{reflect.TypeOf(Use{}), "Cpu", "Count", "spec.Count"},
	} {
		elem := mapElement(t, c.owner, c.field)
		requireField(t, elem, c.wantField, c.wantType)
	}
}

// TestLeasableNeedElementCarriesTheLeaseField pins the half that DESIGN puts in #LeaseFields
// (D-HW-8): the entry of a LEASABLE type says whether it is exclusive. It is generated from the
// same merge, so a regression in either direction fails here.
func TestLeasableNeedElementCarriesTheLeaseField(t *testing.T) {
	for _, owner := range []reflect.Type{reflect.TypeOf(CandyNeed{}), reflect.TypeOf(CandyUse{}), reflect.TypeOf(Need{}), reflect.TypeOf(Use{})} {
		for _, field := range []string{"Cpu", "Memory", "Storage", "Mount"} {
			requireField(t, mapElement(t, owner, field), "Exclusive", "bool")
		}
	}
}
