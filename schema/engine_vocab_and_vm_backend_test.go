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
func TestVmBackendKrunCrossRules(t *testing.T) {
	def, ctx := defValue(t, "#Vm")
	check := checker(def, ctx)

	const containerDisk = `source: {kind: "container_disk", image: "registry.example/disk@sha256:0000000000000000000000000000000000000000000000000000000000000000"}`

	accepted := []string{
		`{backend: "krun", ` + containerDisk + `}`,
		`{backend: "krun", ` + containerDisk + `, krun: {network: "passt", kernel: "image", nested_virt: true, gpu: "virtio", gpu_flags: 962, ram_headroom_mib: 256}}`,
		`{backend: "krun", source: {kind: "bootc", box: "pod-crabbox-systemd"}}`,
	}
	for _, body := range accepted {
		if err := check(body); err != nil {
			t.Errorf("#Vm rejects an accepted krun shape %s: %v", body, err)
		}
	}

	// Every libvirt/qemu-only field must be rejected WITH backend: krun.
	libvirtOnly := []string{
		`libvirt: {}`,
		`autostart: true`,
		`firmware: "uefi-insecure"`,
		`machine: "q35"`,
		`cloud_init: {hostname: "guest"}`,
	}
	for _, field := range libvirtOnly {
		err := check(`{backend: "krun", ` + field + `, ` + containerDisk + `}`)
		if err == nil {
			t.Errorf("#Vm accepts backend: krun with the libvirt-only %s; a krun guest has no libvirt domain", field)
			continue
		}
		// The rejection must NAME the authored field (or, for autostart, the
		// backend rule it violates), not collapse into an anonymous "no arm
		// matches" union error.
		want := strings.SplitN(field, ":", 2)[0]
		if want == "autostart" {
			// The pre-existing autostart:true ⇒ backend auto|libvirt rule fires
			// first and reports at #Vm.backend; the krun conflict is the same
			// rejection, named at the rule that owns it.
			want = "backend"
		}
		if text := allErrors(err); !strings.Contains(text, want) {
			t.Errorf("the krun rejection of %s does not name %q in its error: %s", field, want, text)
		}
	}

	// A non-OCI source arm is rejected too: krun cannot boot a cloud image URL.
	if err := check(`{backend: "krun", source: {kind: "cloud_image", url: "https://example.com/disk.qcow2"}}`); err == nil {
		t.Error("#Vm accepts backend: krun with a cloud_image source; krun requires an OCI-image source")
	}

	// Positive control: the SAME fields are legal with the libvirt backend, so
	// the rejections above are about the backend, not about the fields.
	control := `{backend: "libvirt", libvirt: {}, autostart: true, firmware: "uefi-insecure", machine: "q35", cloud_init: {hostname: "guest"}, ` + containerDisk + `}`
	if err := check(control); err != nil {
		t.Errorf("#Vm rejects the same fields under backend: libvirt (the krun rejection is not backend-specific): %v", err)
	}
}

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

// TestVmKrunDefaultsMaterialize proves the krun defaults are the authoritative
// CUE data, not a consumer-side literal: with the block's fields omitted, CUE
// materializes 962/256 and tsi/stock/off/false. The fields are
// required-with-default inside the optional block, so `krun: {}` is a complete
// set of measured values; a consumer applies the same defaults when the block
// itself is absent.
func TestVmKrunDefaultsMaterialize(t *testing.T) {
	def, ctx := defValue(t, "#Vm")
	const body = `{backend: "krun", source: {kind: "container_disk", image: "registry.example/disk@sha256:0000000000000000000000000000000000000000000000000000000000000000"}, krun: {}}`
	v := def.Unify(ctx.CompileString(body))
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("the krun defaults do not materialize under a concrete validation: %v", err)
	}
	// The fields are required-with-default, so an empty krun block carries the
	// measured values in the value itself — not a consumer-side literal.
	for _, tc := range []struct {
		path string
		want int64
	}{
		{"krun.gpu_flags", 962},
		{"krun.ram_headroom_mib", 256},
	} {
		got, err := v.LookupPath(cue.ParsePath(tc.path)).Int64()
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if got != tc.want {
			t.Errorf("%s = %d, want %d", tc.path, got, tc.want)
		}
	}
	for _, tc := range []struct {
		path, want string
	}{
		{"krun.network", "tsi"},
		{"krun.kernel", "stock"},
		{"krun.gpu", "off"},
	} {
		got, err := v.LookupPath(cue.ParsePath(tc.path)).String()
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.path, got, tc.want)
		}
	}
	if got, err := v.LookupPath(cue.ParsePath("krun.nested_virt")).Bool(); err != nil || got {
		t.Errorf("krun.nested_virt = %v, %v; want false", got, err)
	}
}

// TestDeployTraitsSelectorData pins the DATA half of the hosting rule: a
// substrate declares which deploy selectors it accepts, so a loader can reject
// `engine:` on a vm and `backend:` on a pod without switching on a substrate word.
func TestDeployTraitsSelectorData(t *testing.T) {
	def, ctx := defValue(t, "#DeployTraits")
	check := checker(def, ctx)
	for _, body := range []string{
		`{selectors: ["engine"]}`,
		`{selectors: ["backend"]}`,
		`{selectors: ["engine", "backend"]}`,
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
