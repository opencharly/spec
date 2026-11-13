package spec

// pod_test.go — the DELETION gate for the `kind: pod` body, and the ONE gate that fails if
// the body defs come back.
//
// opencharly/spec#304 unit 3 moved `#Pod` / `#PodValue` into the plugin that serves the kind
// (`plugin-substrate`'s own CUE module, `candy/plugin-substrate/schema/pod.cue`) in leg 1
// (opencharly/plugin-substrate#32), where the plugin's `kind` capability declares
// `InputDef: "#PodValue"`. Every other test in this repo passes identically with the defs
// re-added — the only surface this module still owns for the kind is `#PodSidecar` — so
// without this gate the deletion would be unproven: a re-added `#PodValue` would silently
// re-register `pod` in `KindValueDefs` and the host would gate the body against a def this
// module no longer owns.
//
// The closedness corpus that judged the body moved FIRST, to
// `plugin-substrate/candy/plugin-substrate/schema_serve_test.go` (spec#304 unit 3's leg 1,
// which judges a pod-shaped body ACCEPTED and a typo'd one REJECTED through the composed
// unit), so this leg deletes only what has a live replacement.
//
// The positive control matters as much as the absence: a schema that failed to concatenate
// (or a generator that stopped emitting) would make every absence below "pass" for the wrong
// reason, so a def the base still keeps is asserted PRESENT in the same value.

import (
	"testing"

	"cuelang.org/go/cue"
)

func TestPodBodyDefsAreNotInTheContract(t *testing.T) {
	schema := loadPipelineSchema(t)

	// Positive control: the concatenation and the generator both still work.
	if d := schema.LookupPath(cue.ParsePath("#CandyValue")); d.Err() != nil {
		t.Fatalf("the schema does not carry #CandyValue — the absence checks below would be vacuous: %v", d.Err())
	}
	if def, ok := KindValueDefs["candy"]; !ok || def != "#CandyValue" {
		t.Fatalf("KindValueDefs has no candy entry (got %q, ok=%v) — the generator is not emitting", def, ok)
	}

	// The body defs themselves are gone.
	for _, def := range []string{"#Pod", "#PodValue"} {
		if d := schema.LookupPath(cue.ParsePath(def)); d.Err() == nil {
			t.Errorf("the contract module STILL declares %s — the kind body belongs to plugin-substrate, and a re-added def re-registers the kind in KindValueDefs", def)
		}
	}
	if _, ok := KindValueDefs["pod"]; ok {
		t.Errorf("KindValueDefs still carries %q — deleting #PodValue must drop it from the generated map", "pod")
	}

	// ...and this kind's KEEPER did not move: #PodSidecar stays because the CONTRACT's own
	// #ResolvedPod cites it, and a def a kernel-facing wire shape references cannot live in
	// one plugin's CUE module.
	if d := schema.LookupPath(cue.ParsePath("#PodSidecar")); d.Err() != nil {
		t.Fatalf("the contract module no longer declares #PodSidecar — the contract's own #ResolvedPod cites it: %v", d.Err())
	}
	if d := schema.LookupPath(cue.ParsePath("#ResolvedPod")); d.Err() != nil {
		t.Fatalf("the contract module no longer declares #ResolvedPod — the resolve-to-envelope form the kernel reads: %v", d.Err())
	}
	// The envelope still RESOLVES #PodSidecar, so the keeper is not merely present but wired:
	// a #ResolvedPod carrying one sidecar entry must still validate.
	resolved := schema.LookupPath(cue.ParsePath("#ResolvedPod"))
	probe := schema.Context().CompileString(`sidecar: [{sidecar: {}}]`)
	if err := probe.Err(); err != nil {
		t.Fatalf("compile the #ResolvedPod probe: %v", err)
	}
	if err := resolved.Unify(probe).Validate(cue.Concrete(false)); err != nil {
		t.Fatalf("#ResolvedPod no longer accepts a sidecar entry typed by #PodSidecar: %v", err)
	}
}
