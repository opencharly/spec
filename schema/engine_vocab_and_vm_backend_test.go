package schema_test

import (
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// defValue compiles the shipped schema once and returns the named definition plus
// the CUE context a caller must use to compile its body literals (a value can only
// be unified with another value from the same context).
func defValue(t *testing.T, name string) (cue.Value, *cue.Context) {
	t.Helper()
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath(name))
	if !def.Exists() {
		t.Fatalf("%s is not defined in the shipped schema", name)
	}
	return def, ctx
}

// allErrors flattens CUE's error tree. The top-level Error() truncates to
// "(and N more errors)", so a field-named conflict can be invisible unless every
// node is printed.
func allErrors(err error) string {
	var b strings.Builder
	for _, e := range errors.Errors(err) {
		fmt.Fprintf(&b, "%v | path=%v | ", e.Error(), e.Path())
	}
	return b.String()
}

// checker builds an accept/reject predicate over one definition.
func checker(def cue.Value, ctx *cue.Context) func(string) error {
	return func(body string) error {
		unified := def.Unify(ctx.CompileString(body))
		if unified.Err() != nil {
			return unified.Err()
		}
		return unified.Validate(cue.Concrete(false))
	}
}

// TestEngineNameVocabIsContainerdNotNerdctl is the R5 negative/positive pair for
// the engine word cutover. `containerd` is the engine word (rootful or rootless,
// driven through the nerdctl CLI); `nerdctl` is DELETED as an engine word, so a
// checked-in `engine: nerdctl` must be a schema conflict, not an alias.
func TestEngineNameVocabIsContainerdNotNerdctl(t *testing.T) {
	nameDef, nameCtx := defValue(t, "#EngineName")
	name := checker(nameDef, nameCtx)
	if err := name(`"containerd"`); err != nil {
		t.Fatalf("#EngineName rejects \"containerd\": %v", err)
	}
	if err := name(`"nerdctl"`); err == nil {
		t.Fatal(`#EngineName still accepts "nerdctl"; the engine word must be deleted`)
	}

	// The authored surface is #Deploy.engine. Both arms are asserted there so the
	// rejection is about the authored vocabulary, not only the bare def.
	deployDef, deployCtx := defValue(t, "#Deploy")
	deploy := checker(deployDef, deployCtx)
	if err := deploy(`{engine: "containerd"}`); err != nil {
		t.Fatalf("#Deploy rejects engine: containerd: %v", err)
	}
	if err := deploy(`{engine: "nerdctl"}`); err == nil {
		t.Fatal("#Deploy accepts engine: nerdctl; the deleted engine word must be rejected")
	}
	// Control: a genuinely unknown word is rejected by the same gate, so the
	// positive arm above is not vacuous.
	if err := deploy(`{engine: "bogus"}`); err == nil {
		t.Fatal("#Deploy accepts engine: bogus; #EngineName is not closed")
	}
}

// TestVmBackendKrunCrossRules pins the krun backend contract: a krun guest's root
// is a containerd snapshot, so `source` must be an OCI-image arm (container_disk
// or bootc) and the libvirt/qemu-only fields are rejected. The libvirt control at
// the end proves the rejection is krun-specific, not the fields being invalid.

// TestVmDeployStateBackendVocabulary pins the persisted-state arm: the recorded
// backend may be any selector/backend word, and krun is now one of them.
func TestVmDeployStateBackendVocabulary(t *testing.T) {
	def, ctx := defValue(t, "#VmDeployState")
	check := checker(def, ctx)
	for _, word := range []string{"auto", "libvirt", "qemu", "krun"} {
		if err := check(`{backend: "` + word + `"}`); err != nil {
			t.Errorf("#VmDeployState rejects backend %q: %v", word, err)
		}
	}
	if err := check(`{backend: "bogus"}`); err == nil {
		t.Error("#VmDeployState accepts backend: bogus; the backend vocabulary is not closed")
	}
}

// TestDeployBackendAndNamespace pins the deploy-entity additions: a per-deploy
// VM-backend override and a containerd namespace override are authored fields,
// with the backend word list closed.
func TestDeployBackendAndNamespace(t *testing.T) {
	def, ctx := defValue(t, "#Deploy")
	check := checker(def, ctx)
	if err := check(`{from: "some-vm", backend: "krun", namespace: "default"}`); err != nil {
		t.Fatalf("#Deploy rejects backend/namespace on a vm deploy: %v", err)
	}
	if err := check(`{from: "some-vm", backend: "bogus"}`); err == nil {
		t.Error("#Deploy accepts backend: bogus; #VmBackend is not closed")
	}
}

// TestDeployRootfulVocabulary pins the per-deploy rootful selector and its closed
// vocabulary. `rootful:` is an authored deploy field (the W3.1 selection model),
// so a typo must be a schema conflict, not a value that reaches a resolver.
func TestDeployRootfulVocabulary(t *testing.T) {
	def, ctx := defValue(t, "#Deploy")
	check := checker(def, ctx)
	for _, mode := range []string{"auto", "machine", "sudo", "native"} {
		if err := check(`{rootful: "` + mode + `"}`); err != nil {
			t.Errorf("#Deploy rejects rootful: %q: %v", mode, err)
		}
	}
	if err := check(`{rootful: "bogus"}`); err == nil {
		t.Error("#Deploy accepts rootful: bogus; #RootfulMode is not closed")
	}

	// The bare def is closed too, so the rejection is the vocabulary's, not a
	// quirk of the authored field.
	modeDef, modeCtx := defValue(t, "#RootfulMode")
	mode := checker(modeDef, modeCtx)
	if err := mode(`"bogus"`); err == nil {
		t.Error("#RootfulMode accepts \"bogus\"; the rootful vocabulary is not closed")
	}
}

// TestVmBackendNameIsProviderWordsOnly pins the split the resolver depends on:
// #VmBackendName is the PROVIDER vocabulary (libvirt/qemu/krun) that cue-gen
// emits as spec.VmBackends, while #VmBackend adds `auto` as the resolution
// default. `auto` must never leak into the provider list.
func TestVmBackendNameIsProviderWordsOnly(t *testing.T) {
	def, ctx := defValue(t, "#VmBackendName")
	check := checker(def, ctx)
	for _, word := range []string{"libvirt", "qemu", "krun"} {
		if err := check(`"` + word + `"`); err != nil {
			t.Errorf("#VmBackendName rejects %q: %v", word, err)
		}
	}
	for _, word := range []string{"auto", "bogus"} {
		if err := check(`"` + word + `"`); err == nil {
			t.Errorf("#VmBackendName accepts %q; the provider vocabulary excludes the auto selector", word)
		}
	}

	// The authored/persisted selector still carries auto as its default.
	fullDef, fullCtx := defValue(t, "#VmBackend")
	full := checker(fullDef, fullCtx)
	if err := full(`"auto"`); err != nil {
		t.Errorf("#VmBackend rejects \"auto\": %v", err)
	}
}

// TestVmKrunDefaultsMaterialize proves the krun defaults are the authoritative
// CUE data, not a consumer-side literal: with the block's fields omitted, CUE
// materializes 962/256 and tsi/stock/off/false. The fields are
// required-with-default inside the optional block, so `krun: {}` is a complete
// set of measured values; a consumer applies the same defaults when the block
// itself is absent.

// TestDeployTraitsSelectorData pins the DATA half of the hosting rule: a
// substrate declares which deploy selectors it accepts, so a loader can reject
// `engine:` on a vm and `backend:` on a pod without switching on a substrate word.
func TestDeployTraitsSelectorData(t *testing.T) {
	def, ctx := defValue(t, "#DeployTraits")
	check := checker(def, ctx)
	for _, body := range []string{
		`{selectors: ["engine"]}`,
		`{selectors: ["backend"]}`,
		`{selectors: ["rootful"]}`,
		`{selectors: ["engine", "backend", "rootful"]}`,
		`{}`,
	} {
		if err := check(body); err != nil {
			t.Errorf("#DeployTraits rejects %s: %v", body, err)
		}
	}
	if err := check(`{selectors: ["bogus"]}`); err == nil {
		t.Error("#DeployTraits accepts an unknown selector word; the selector vocabulary is not closed")
	}
}
