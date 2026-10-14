package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/spec"
)

// TestKindValueDefs_HasFirecracker gates the schemagen-derived word→def map: the new
// substrate kind must be auto-derived into spec.KindValueDefs (the host value gate
// consults it) from the #FirecrackerValue def — no hand-maintained map entry.
func TestKindValueDefs_HasFirecracker(t *testing.T) {
	def, ok := spec.KindValueDefs["firecracker"]
	if !ok || def != "#FirecrackerValue" {
		t.Fatalf("KindValueDefs[firecracker] = %q (ok=%v), want #FirecrackerValue", def, ok)
	}
}

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

// TestFirecrackerValue_TemplateAccepted passes a realistic kind:firecracker template
// (the host surface + a microVM instance + a runner pool) through the host-side value
// gate def #FirecrackerValue, proving the new authored surface validates.
func TestFirecrackerValue_TemplateAccepted(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#FirecrackerValue"))
	if !def.Exists() {
		t.Fatal("#FirecrackerValue is not defined in the shipped schema")
	}
	body := `{
	vmm: {version: "v1.17.0", sha256: "06094a1108ae9e82aa4c23a775aa92758f53f1175d422270d9d6162cb9ade558", jailer: {enabled: false}}
	kernel: {from: "ghcr.io/opencharly/firecracker-kernel:v0.1", cmdline: "console=ttyS0 reboot=k panic=1 pci=off nomodules rw"}
	network: {
		mode: "tap"
		tap: {name: "fc-tap{n}", subnet: "172.16.0.0/16", prefix: 30, host_offset: 1}
		egress_iface: "auto"
		forwarding: {ip_forward: true, policy: "append-accept"}
		resolver: ["1.1.1.1", "8.8.8.8"]
	}
	containerd: {
		snapshotter: "blockfile"
		manage: "merge"
		devmapper: {pool: "containerd-thinpool", base_image_size: "30GB", device: "/dev/nvme1n1", profile: {thin_pool_autoextend_threshold: 80}}
	}
	fireactions: {
		version: "v2.0.8"
		install: {from_source: true}
		bind_address: "127.0.0.1:8080"
		github: {app_id: 123456, app_private_key_secret: "charly/fireactions/app-key"}
		pools: [{
			name: "opencharly-2vcpu-4gb"
			replicas: 2
			runner: {image: "ghcr.io/opencharly/fireactions-runner:v0.1", organization: "opencharly", group_id: 1, labels: ["self-hosted", "fireactions"]}
			firecracker: {machine_config: {vcpu_count: 2, mem_size_mib: 4096}, metadata: {pool: "opencharly-2vcpu-4gb"}}
		}]
	}
	image: {from: "ghcr.io/opencharly/fireactions-runner:v0.1", format: "ext4", disk_mib: 6144, resize: true}
	machine: {
		vcpu_count: 2
		mem_size_mib: 4096
		drives: [{drive_id: "work", path: "/var/lib/firecracker/work.ext4", io_engine: "Async"}]
		rate_limits: {network_out: {bandwidth: {size: 26214400, refill_time: 1000, one_time_burst: 0}}}
	}
	mmds: {version: "V2", metadata: {charly: {deploy: "runner-vm"}}}
	cloud_init: {hostname: "fc-1", package: ["sudo"]}
	vsock: {guest_cid: 3}
	console: {serial_out_path: "/var/log/fc-1.serial", log_level: "Info"}
}`
	in := cuecontext.New().CompileString(body)
	if err := in.Unify(def).Validate(); err != nil {
		t.Fatalf("a valid kind:firecracker template rejected by #FirecrackerValue: %v", err)
	}
}

// TestFirecrackerValue_UnknownKeyRejected proves the new def is CLOSED (a typo is an
// error, not a silent drop) — the closedness the host value gate relies on.
func TestFirecrackerValue_UnknownKeyRejected(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#FirecrackerValue"))
	body := `{kernel: {path: "/boot/vmlinux"}, image: {from: "x"}, bogus_key: 1}`
	in := cuecontext.New().CompileString(body)
	if err := in.Unify(def).Validate(); err == nil {
		t.Fatal("a kind:firecracker template with an unknown key must fail the closed gate")
	}
}

// TestFirecrackerValue_SnapshotterEnumRejected proves the enum'd sub-shapes carry
// their own strictness: a snapshotter outside the two that can back a root drive is
// refused by the schema, not left to the plugin — and the two legal values still
// pass, so the gate is the enum and not a blanket rejection.
func TestFirecrackerValue_SnapshotterEnumRejected(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#FirecrackerValue"))

	for _, ok := range []string{"devmapper", "blockfile"} {
		body := `{kernel: {path: "/boot/vmlinux"}, image: {from: "x"}, containerd: {snapshotter: "` + ok + `"}}`
		in := cuecontext.New().CompileString(body)
		if err := in.Unify(def).Validate(); err != nil {
			t.Fatalf("containerd.snapshotter %q must be accepted: %v", ok, err)
		}
	}

	for _, bad := range []string{"overlayfs", "native"} {
		body := `{kernel: {path: "/boot/vmlinux"}, image: {from: "x"}, containerd: {snapshotter: "` + bad + `"}}`
		in := cuecontext.New().CompileString(body)
		if err := in.Unify(def).Validate(); err == nil {
			t.Fatalf("containerd.snapshotter %q must fail: its mount is a directory, which cannot back a microVM root drive", bad)
		}
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
// — the field is optional, so a required-field check would not exercise the enum.
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
