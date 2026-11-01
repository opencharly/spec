package spec

import (
	"reflect"
	"testing"
)

// The inner-parallelism knob named the engine it was handed to (`podman build --jobs N`).
// With one builder there is no second engine to name, so the knob is renamed rather than
// deleted: `stage_parallelism` bounds the STAGES of a single build, while `jobs` keeps its
// own name and bounds the OUTER dimension (images per DAG level). The renamed wire names are
// part of the same contract, so the renamed field must be PRESENT on every type that carries
// the knob.
//
// The ABSENCE of the old names is asserted by the contract's list-driven guard
// (`retired.Identifiers`): the canonical retired-identifier list is where the withdrawn names
// live, and one guard reads that list, so a withdrawn name is asserted in one place rather
// than restated per type.
func TestStageParallelismRenameAcrossTheContract(t *testing.T) {
	authored := []struct {
		typ    reflect.Type
		fields []string
	}{
		{reflect.TypeOf(Box{}), []string{"StageParallelism", "StageParallelismCap"}},
	}
	for _, c := range authored {
		for _, field := range c.fields {
			if _, ok := c.typ.FieldByName(field); !ok {
				t.Errorf("%s must carry the renamed field %q", c.typ, field)
			}
		}
	}

	// The OUTER knob keeps its own name: it is not part of this rename, and the
	// rename must not have taken it with it.
	if _, ok := reflect.TypeOf(Box{}).FieldByName("Jobs"); !ok {
		t.Error("Box must keep the outer knob Jobs — the inner rename must not have taken it")
	}

	// The three build WIRE envelopes carry the same knob across the plugin boundary;
	// the renamed wire field must be on each of them.
	for _, typ := range []reflect.Type{
		reflect.TypeOf(BuildResolveRequest{}),
		reflect.TypeOf(BuildResolveReply{}),
		reflect.TypeOf(BuildRequest{}),
	} {
		if _, ok := typ.FieldByName("StageParallelism"); !ok {
			t.Errorf("%s must carry the renamed wire field StageParallelism", typ)
		}
	}
}
