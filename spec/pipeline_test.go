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

import "testing"

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
