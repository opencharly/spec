package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/spec"
)

// TestKindValueDefs_HasKubevirt gates the schemagen-derived word→def map: the new
// 6th substrate kind must be auto-derived into spec.KindValueDefs (the host value
// gate consults it) from the #KubevirtValue def — no hand-maintained map entry.
func TestKindValueDefs_HasKubevirt(t *testing.T) {
	def, ok := spec.KindValueDefs["kubevirt"]
	if !ok || def != "#KubevirtValue" {
		t.Fatalf("KindValueDefs[kubevirt] = %q (ok=%v), want #KubevirtValue", def, ok)
	}
}

// TestResourceKinds_HasKubevirt gates the schemagen-derived deployable-kind
// vocabulary: kubevirt must be a #ResourceKind (so the loader nests its members
// and the Go side derives the deploy-target vocabulary from it).
func TestResourceKinds_HasKubevirt(t *testing.T) {
	found := false
	for _, w := range spec.ResourceKinds {
		if w == "kubevirt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("spec.ResourceKinds = %v, want it to contain kubevirt", spec.ResourceKinds)
	}
}

// TestKubevirtValue_TemplateAccepted passes a realistic kind:kubevirt template
// (container_disk boot + cloud-init + a network + a GPU) through the host-side
// value gate def #KubevirtValue, proving the new authored surface validates.
func TestKubevirtValue_TemplateAccepted(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#KubevirtValue"))
	if !def.Exists() {
		t.Fatal("#KubevirtValue is not defined in the shipped schema")
	}
	body := `{
	cluster: "prod"
	namespace: "vms"
	source: {kind: "container_disk", image: "quay.io/example/vmbox:v1", pull_policy: "IfNotPresent"}
	memory: "4G"
	cpu: {cores: 2, sockets: 1}
	firmware: {bootloader: "efi", efi_secure_boot: false}
	devices: {rng: true, autoattach_serial_console: true}
	gpus: [{resource_name: "nvidia.com/gpu"}]
	network: {interface: "masquerade", model: "virtio", ports: [{name: "ssh", port: 22, protocol: "TCP"}]}
	run_strategy: "Always"
	cloud_init: {hostname: "kv1", package: ["sudo"]}
}`
	in := cuecontext.New().CompileString(body)
	if err := in.Unify(def).Validate(); err != nil {
		t.Fatalf("a valid kind:kubevirt template rejected by #KubevirtValue: %v", err)
	}
}

// TestKubevirtValue_UnknownKeyRejected proves the new def is CLOSED (a typo is an
// error, not a silent drop) — the closedness the host value gate relies on.
func TestKubevirtValue_UnknownKeyRejected(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#KubevirtValue"))
	body := `{source: {kind: "container_disk", image: "x"}, bogus_key: 1}`
	in := cuecontext.New().CompileString(body)
	if err := in.Unify(def).Validate(); err == nil {
		t.Fatal("a kind:kubevirt template with an unknown key must fail the closed gate")
	}
}
