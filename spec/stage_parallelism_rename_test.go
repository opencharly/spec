package spec

import (
	"reflect"
	"testing"
)

// The inner-parallelism knob named the engine it was handed to (`podman build --jobs N`).
// With one builder there is no second engine to name, so the knob is renamed rather than
// deleted: `stage_parallelism` bounds the STAGES of a single build, while `jobs` keeps its
// own name and bounds the OUTER dimension (images per DAG level). The renamed wire names are
// part of the same contract, so the old name must be gone from every one of these types.
//
// This is the fail-without instrument for the Go half of the rename: it FAILS while the
// generated types still carry the engine-named fields.
func TestStageParallelismRenameAcrossTheContract(t *testing.T) {
	// POSITIVE CONTROL first: the instrument finds fields that ARE there, so the
	// absence assertions below cannot pass vacuously.
	if _, ok := reflect.TypeOf(Box{}).FieldByName("Jobs"); !ok {
		t.Fatal("control failed: Box.Jobs not found — the instrument is broken, not the contract")
	}

	authored := []struct {
		typ      reflect.Type
		want     [2]string // {new field, old field}
		optional []string
	}{
		{reflect.TypeOf(Box{}), [2]string{"StageParallelism", "PodmanJobs"}, []string{"StageParallelismCap", "PodmanJobsCap"}},
	}
	for _, c := range authored {
		if _, ok := c.typ.FieldByName(c.want[0]); !ok {
			t.Errorf("%s must carry the renamed field %q", c.typ, c.want[0])
		}
		if f, ok := c.typ.FieldByName(c.want[1]); ok {
			t.Errorf("%s must not carry the engine-named field %q (yaml %q)", c.typ, f.Name, f.Tag.Get("yaml"))
		}
		for i := 0; i+1 < len(c.optional); i += 2 {
			if _, ok := c.typ.FieldByName(c.optional[i]); !ok {
				t.Errorf("%s must carry the renamed field %q", c.typ, c.optional[i])
			}
			if f, ok := c.typ.FieldByName(c.optional[i+1]); ok {
				t.Errorf("%s must not carry the engine-named field %q (yaml %q)", c.typ, f.Name, f.Tag.Get("yaml"))
			}
		}
	}

	// The three build WIRE envelopes carry the same knob across the plugin boundary;
	// the old name must be gone from each, or the contract would keep a dead engine's
	// name on the wire (R5).
	for _, typ := range []reflect.Type{
		reflect.TypeOf(BuildResolveRequest{}),
		reflect.TypeOf(BuildResolveReply{}),
		reflect.TypeOf(BuildRequest{}),
	} {
		if _, ok := typ.FieldByName("StageParallelism"); !ok {
			t.Errorf("%s must carry the renamed wire field StageParallelism", typ)
		}
		if f, ok := typ.FieldByName("PodmanJobs"); ok {
			t.Errorf("%s must not carry the engine-named wire field %q (yaml %q)", typ, f.Name, f.Tag.Get("yaml"))
		}
	}
}
