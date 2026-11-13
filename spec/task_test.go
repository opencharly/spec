package spec

// task_test.go — the DELETION gate for the `kind: task` body, and the ONE gate that fails
// if the body defs come back.
//
// opencharly/spec#304 unit 2 moved `#Task` / `#TaskValue` into the plugin that serves the
// kind (`plugin-task`'s own CUE module, `candy/plugin-task/schema/task.cue`). Every other
// test in this repo passes identically with the defs re-added — the surfaces this module
// still owns for the kind are the SHARED parameter contract, which did not move — so
// without this gate the deletion would be unproven: a re-added `#TaskValue` would silently
// re-register `task` in `KindValueDefs` and the host would gate the body against a def this
// module no longer owns.
//
// The corpus that judged the body moved FIRST, to
// `plugin-task/candy/plugin-task/task_schema_corpus_test.go` (spec#304 unit 2's leg 1), so
// this file deletes only what has a live replacement.
//
// The positive control matters as much as the absence: a schema that failed to concatenate
// (or a generator that stopped emitting) would make every absence below "pass" for the wrong
// reason, so a def the base still keeps is asserted PRESENT in the same value.

import (
	"testing"

	"cuelang.org/go/cue"
)

func TestTaskBodyDefsAreNotInTheContract(t *testing.T) {
	schema := loadPipelineSchema(t)

	// Positive control: the concatenation and the generator both still work.
	if d := schema.LookupPath(cue.ParsePath("#CandyValue")); d.Err() != nil {
		t.Fatalf("the schema does not carry #CandyValue — the absence checks below would be vacuous: %v", d.Err())
	}
	if def, ok := KindValueDefs["candy"]; !ok || def != "#CandyValue" {
		t.Fatalf("KindValueDefs has no candy entry (got %q, ok=%v) — the generator is not emitting", def, ok)
	}

	// The body defs themselves are gone.
	for _, def := range []string{"#Task", "#TaskValue"} {
		if d := schema.LookupPath(cue.ParsePath(def)); d.Err() == nil {
			t.Errorf("the contract module STILL declares %s — the kind body belongs to plugin-task, and a re-added def re-registers the kind in KindValueDefs", def)
		}
	}
	if _, ok := KindValueDefs["task"]; ok {
		t.Errorf("KindValueDefs still carries %q — deleting #TaskValue must drop it from the generated map", "task")
	}

	// ...and the SHARED half did NOT move: `#TaskParamSpec` stays, because
	// `plugin-pipeline`'s `args:` references it and a plugin can import only this module.
	if d := schema.LookupPath(cue.ParsePath("#TaskParamSpec")); d.Err() != nil {
		t.Fatalf("the contract module no longer declares #TaskParamSpec — it is SHARED with `kind: pipeline`'s args:, which can only reference this module: %v", d.Err())
	}
}
