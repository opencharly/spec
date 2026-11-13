package spec

// pipeline_test.go — the workflow VOCABULARY gates that outlive the `kind: pipeline`
// body's move out of this module.
//
// The body defs (`#Pipeline` / `#PipelineValue`, their step/trigger/flow helpers and the
// on_error vocabulary table) lived in `schema/pipeline.cue` until unit 1 of
// opencharly/spec#304
// moved them to the plugin that serves the kind (`plugin-pipeline`'s own CUE module).
// Their closedness corpus went with them, to
// `plugin-pipeline/candy/plugin-pipeline/pipeline_schema_corpus_test.go`; deleting it
// here without moving it would have been a silent coverage loss.
//
// What remains is about the VOCABULARY, not the body: `#PipelineClassRoutes` is a table
// over this module's own `#ProviderClassNames`, and it stays here because no plugin sees
// the whole class vocabulary and no plugin can run this module's vocabulary generator.
// Both gates below read the GENERATED Go vocabulary (`spec/vocab_gen.go`), so they fail
// if the CUE table and the class set drift in either direction.

import (
	"testing"

	"cuelang.org/go/cue"
)

// TestPipelineBodyDefsAreNotInTheContract pins the DELETION itself: the `kind: pipeline`
// body lives in plugin-pipeline's CUE module now, so the contract module must not declare
// it. This is the one gate that FAILS if a body def is re-added — every other test in this
// repo passes identically with the defs back, because they exercise surfaces this module
// still owns (the class→route table, the vocabularies). Without it the deletion would be
// unproven: a re-added `#PipelineValue` would silently re-register `pipeline` in
// `KindValueDefs` and the host would gate the body against a def this module no longer owns.
//
// The positive control matters as much as the absence: a schema that failed to concatenate
// (or a generator that stopped emitting) would make every absence below "pass" for the wrong
// reason, so a def the base still keeps is asserted PRESENT in the same value.
func TestPipelineBodyDefsAreNotInTheContract(t *testing.T) {
	schema := loadPipelineSchema(t)

	// Positive control: the concatenation and the generator both still work. (It used to
	// assert `#TaskValue`; opencharly/spec#304 unit 2 moved the `task` body to its plugin
	// too, so the control is now `#CandyValue` — a def this module OWNS and keeps.)
	if d := schema.LookupPath(cue.ParsePath("#CandyValue")); d.Err() != nil {
		t.Fatalf("the schema does not carry #CandyValue — the absence checks below would be vacuous: %v", d.Err())
	}
	if def, ok := KindValueDefs["candy"]; !ok || def != "#CandyValue" {
		t.Fatalf("KindValueDefs has no candy entry (got %q, ok=%v) — the generator is not emitting", def, ok)
	}

	for _, def := range []string{
		"#Pipeline",
		"#PipelineValue",
		"#PipelineRetry",
		"#PipelineApproval",
		"#PipelineFlow",
		"#PipelineArms",
		"#PipelineInput",
		"#PipelineSubStep",
		"#PipelineParallel",
		"#PipelineStepBase",
		"#PipelineStep",
		"#PipelineSchedule",
		"#PipelineTrigger",
	} {
		if d := schema.LookupPath(cue.ParsePath(def)); d.Err() == nil {
			t.Errorf("the contract module STILL declares %s — the kind body belongs to plugin-pipeline, and a re-added def re-registers the kind in KindValueDefs", def)
		}
	}
	if _, ok := KindValueDefs["pipeline"]; ok {
		t.Errorf("KindValueDefs still carries %q — deleting #PipelineValue must drop it from the generated map", "pipeline")
	}
}

// TestPipelineClassCoverage is the R2 gate: every provider class in the CLOSED vocabulary
// must have a decided workflow route, and the route table must not name a class that does
// not exist. Both directions matter — a one-way check would let a stale row outlive the
// class it describes. It lives in spec because the table IS spec data; plugin-pipeline
// re-runs it against its lowering in W5.
func TestPipelineClassCoverage(t *testing.T) {
	if len(ProviderClasses) == 0 {
		t.Fatal("ProviderClasses is empty — schemagen did not emit the vocabulary")
	}
	for _, class := range ProviderClasses {
		if PipelineClassRoutes[class] == "" {
			t.Errorf("provider class %q has no workflow route — adding a class MUST force the route decision (spec.PipelineClassRoutes)", class)
		}
	}
	inVocab := make(map[string]bool, len(ProviderClasses))
	for _, class := range ProviderClasses {
		inVocab[class] = true
	}
	for class := range PipelineClassRoutes {
		if !inVocab[class] {
			t.Errorf("PipelineClassRoutes names %q, which is not in ProviderClasses — the table has drifted from the vocabulary", class)
		}
	}
}

// TestKindAndClassVocabulariesAreDisjoint pins the KIND-vs-CLASS split structurally.
//
// KindValueDefs is DERIVED from the `#<X>Value` def names, and each of those defs is the
// host-side value gate for an AUTHORABLE ENTITY KIND — a word some plugin serves as
// `kind: <word>`. ProviderClasses is a DISPATCH vocabulary: the faces a plugin can be
// invoked through. The two are different axes of the same plugin, and a word that appears
// in BOTH is a defect with a specific shape: it makes `kind: <word>` gateable while
// `<word>:` dispatches to a plugin, so the host validates a kind no plugin actually serves.
//
// This is not hypothetical — it is the exact bug this change shipped once and then
// removed: a `#WorkflowValue` def (added by analogy with `#PipelineValue`, without noticing
// that `pipeline` is a plugin-SERVED KIND while `workflow` is a provider CLASS) silently
// registered `workflow` in KindValueDefs. The generated map cannot show that on its own,
// which is why the gate is here rather than in a comment: a decode-only or generation-only
// check passes either way.
func TestKindAndClassVocabulariesAreDisjoint(t *testing.T) {
	if len(KindValueDefs) == 0 || len(ProviderClasses) == 0 {
		t.Fatal("a vocabulary is empty — schemagen did not emit one of them")
	}
	for kind := range KindValueDefs {
		for _, class := range ProviderClasses {
			if kind == class {
				t.Errorf("%q is BOTH a kind (KindValueDefs) and a provider class (ProviderClasses): "+
					"a kind is a word a plugin SERVES as `kind: %s`, a class is a face it is DISPATCHED through — "+
					"a word in both makes `kind: %s` gateable against a def for a kind no plugin serves", kind, kind, kind)
			}
		}
	}
}
