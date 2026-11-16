package schema_test

// vm_defaults_test.go — the gate on opencharly/spec#341: `#ResolvedVm.backend` and
// `#ResolvedVm.autostart` must be OPTIONAL, not required.
//
// The distinction is load-bearing rather than cosmetic. The SDK's kind table maps
// `vm` to `#ResolvedVm` (loaderkit: cueKindDefs["vm"]), and plugin-vm routes every
// `charly vm create` through `loaderkit.ApplyCueDefaults("vm", …)`, which can only
// materialize a required-WITH-default field. With plain `backend!: string` and
// `autostart!: bool`, a VM entity that omits them (which every entity in the org did,
// because the defaults used to be filled in) failed to decode:
//
//	applyCueDefaults vm: decode: backend: cannot convert non-concrete value string
//
// So this test asserts the property that failure depends on: a `#ResolvedVm` whose
// backend/autostart were omitted DECODES. It fails on the pre-fix (`!:`) schema with
// the production error, and passes on the optional one.

import (
	"testing"

	"cuelang.org/go/cue"
)

// resolvedVmWithMinimalSource returns `#ResolvedVm` constrained by the minimum an
// authored cloud_image VM must supply, filled through the shipped value's OWN
// context (a cue.Value only interoperates with values from the context that built
// it, so the entity cannot be compiled separately).
func resolvedVmWithMinimalSource(t *testing.T, shipped cue.Value) cue.Value {
	t.Helper()
	v := shipped.FillPath(cue.ParsePath("#ResolvedVm.source.kind"), "cloud_image")
	v = v.FillPath(cue.ParsePath("#ResolvedVm.source.url"), "https://example.invalid/x.img")
	def := v.LookupPath(cue.ParsePath("#ResolvedVm"))
	if def.Err() != nil {
		t.Fatalf("lookup #ResolvedVm: %v", def.Err())
	}
	return def
}

func TestResolvedVmBackendAndAutostartCarryDefaults(t *testing.T) {
	def := resolvedVmWithMinimalSource(t, compileShipped(t))

	// The claim: an entity that says nothing about either field decodes, and gets the
	// schema's own defaults. Without the required-with-default declaration this Decode
	// fails with "cannot convert non-concrete value", which is precisely the org-wide
	// `charly vm create` breakage this guards.
	var out struct {
		Backend   string
		Autostart bool
	}
	if err := def.Decode(&out); err != nil {
		t.Fatalf("decode #ResolvedVm without backend/autostart: %v\n"+
			"This is the opencharly/spec#341 regression: these two fields must be OPTIONAL "+
			"(`backend?: string`, `autostart?: bool`) so an envelope whose producer omitted "+
			"their zero values still decodes. A required-with-default spelling does NOT fix "+
			"it - Decode never resolves a default disjunction - which this test also covers.", err)
	}
	// Absent is the correct post-decode state for an envelope whose producer omitted them:
	// the Go zero values. The regression was that this DECODE FAILED at all.
	if out.Backend != "" {
		t.Fatalf("backend = %q, want the empty zero value for an omitted field", out.Backend)
	}
	if out.Autostart {
		t.Fatalf("autostart = %v, want false", out.Autostart)
	}
}

func TestResolvedVmBackendAcceptsEveryProviderWord(t *testing.T) {
	// The default must not narrow the authored vocabulary: a caller that DOES name a
	// provider word still validates, or the fix would have traded one breakage for
	// another.
	for _, word := range []string{"libvirt", "qemu"} {
		shipped := compileShipped(t)
		v := shipped.FillPath(cue.ParsePath("#ResolvedVm.source.kind"), "cloud_image")
		v = v.FillPath(cue.ParsePath("#ResolvedVm.source.url"), "https://example.invalid/x.img")
		v = v.FillPath(cue.ParsePath("#ResolvedVm.backend"), word)
		def := v.LookupPath(cue.ParsePath("#ResolvedVm"))
		var out struct {
			Backend string
		}
		if err := def.Decode(&out); err != nil {
			t.Fatalf("backend %q rejected: %v", word, err)
		}
		if out.Backend != word {
			t.Fatalf("backend = %q, want %q", out.Backend, word)
		}
	}
}
