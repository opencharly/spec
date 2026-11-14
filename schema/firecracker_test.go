package schema_test

// firecracker_test.go — the firecracker-adjacent assertions that belong to the CONTRACT.
//
// opencharly/spec#304 unit 7 moved the `kind: firecracker` BODY out of this module (into the
// plugin that serves the kind), so the body's own five validation tests moved with it
// (opencharly/plugin-substrate#41) and the `#FirecrackerValue` def is no longer declared here.
// What remains are the assertions about the VOCABULARY, which the contract still owns: the
// deployable-kind word list, the `#SubstrateKind` enum (gated against the Go consts so the two
// sides cannot drift) and the `#Deploy.target` enum.

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/spec"
)

// TestResourceKinds_HasFirecracker gates the schemagen-derived deployable-kind
// vocabulary: firecracker must be a #ResourceKind (so the loader nests its members
// and the Go side derives the deploy-target vocabulary from it).
func TestResourceKinds_HasFirecracker(t *testing.T) {
	found := false
	for _, w := range spec.ResourceKinds {
		if w == "firecracker" {
			found = true
		}
	}
	if !found {
		t.Fatalf("spec.ResourceKinds = %v, want it to contain firecracker", spec.ResourceKinds)
	}
}

// TestSubstrateKind_GoConstsMatchTheCueEnum gates the two sides of the substrate
// discriminator against EACH OTHER: every hand-written spec.SubstrateKind const must
// be accepted by the CUE enum #SubstrateKind. kubevirt's const existed in Go while
// the CUE enum did not list it — exactly the drift this test now catches, for
// kubevirt and firecracker alike.
func TestSubstrateKind_GoConstsMatchTheCueEnum(t *testing.T) {
	v := compileShipped(t)
	enumDef := v.LookupPath(cue.ParsePath("#SubstrateKind"))
	if !enumDef.Exists() {
		t.Fatal("#SubstrateKind is not defined in the shipped schema")
	}
	for _, w := range []spec.SubstrateKind{
		spec.SubstratePod,
		spec.SubstrateVM,
		spec.SubstrateKubernetes,
		spec.SubstrateLocal,
		spec.SubstrateAndroid,
		spec.SubstrateKindcluster,
		spec.SubstrateKubevirt,
		spec.SubstrateFirecracker,
	} {
		doc := cuecontext.New().CompileString(`"` + string(w) + `"`)
		if err := doc.Unify(enumDef).Validate(); err != nil {
			t.Fatalf("the Go const %q is not accepted by the CUE #SubstrateKind enum: %v", w, err)
		}
	}
}

// TestDeployTarget_HasKubevirtAndFirecracker gates the loader-derived target enum on
// #Deploy: both substrates must be pinnable, which is the residue left behind when
// kubevirt was added (its target word was never added to the enum). The values are
// checked by unifying a target-only document against #Deploy with cue.Concrete(false)
// — the field is optional, so a completeness check would test the wrong thing.
func TestDeployTarget_HasKubevirtAndFirecracker(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#Deploy"))
	if !def.Exists() {
		t.Fatal("#Deploy is not defined in the shipped schema")
	}
	for _, w := range []string{"kubevirt", "firecracker"} {
		doc := cuecontext.New().CompileString(`{target: "` + w + `"}`)
		if err := doc.Unify(def).Validate(cue.Concrete(false)); err != nil {
			t.Fatalf("target %q rejected by the #Deploy target enum: %v", w, err)
		}
	}
	// The enum must still refuse a word outside it — otherwise the gate above would
	// pass on an open string.
	bad := cuecontext.New().CompileString(`{target: "not-a-substrate"}`)
	if err := bad.Unify(def).Validate(cue.Concrete(false)); err == nil {
		t.Fatal("the #Deploy target enum must reject a word outside it")
	}
}
