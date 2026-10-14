package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// vmBoxDef returns a shipped def, failing the test when it is absent.
func vmBoxDef(t *testing.T, name string) cue.Value {
	t.Helper()

	def := compileShipped(t).LookupPath(cue.ParsePath(name))
	if !def.Exists() {
		t.Fatalf("%s is not defined in the shipped schema", name)
	}

	return def
}

// accept asserts that a document validates against the def AND is complete — the
// completeness half is what makes the gate able to fail: plain Validate() treats a
// missing required field as incomplete rather than erroneous.
func accept(t *testing.T, def cue.Value, body string) {
	t.Helper()

	if err := cuecontext.New().CompileString(body).Unify(def).Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("a valid document was rejected: %v", err)
	}
}

// reject asserts that a document does NOT validate — the closed / enum half.
func reject(t *testing.T, def cue.Value, body, why string) {
	t.Helper()

	if err := cuecontext.New().CompileString(body).Unify(def).Validate(); err == nil {
		t.Fatalf("%s", why)
	}
}

// TestVmBoxSource_MicroVMArmsAccepted gates the two arms this change adds: a microVM
// rootfs (the ext4/raw file the VMM attaches) and the kernel it boots with. Each arm
// carries its own provenance — the origin image, the size the rootfs was built at, the
// kernel's digest, release and command line.
func TestVmBoxSource_MicroVMArmsAccepted(t *testing.T) {
	def := vmBoxDef(t, "#VmBoxSource")

	accept(t, def, `{
	kind: "rootfs"
	image: "ghcr.io/opencharly/fireactions-runner:v0.1"
	format: "ext4"
	disk_mib: 6144
}`)

	accept(t, def, `{
	kind: "kernel"
	image: "ghcr.io/opencharly/firecracker-kernel:v0.1"
	format: "vmlinux"
	sha256: "06094a1108ae9e82aa4c23a775aa92758f53f1175d422270d9d6162cb9ade558"
	release: "6.1.155"
	cmdline: "console=ttyS0 reboot=k panic=1 pci=off nomodules rw ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off"
}`)
}

// TestVmBoxSource_DiskOnlyArmsUnchanged gates that the six pre-existing arms still
// validate, so widening the kind space did not disturb the shapes already in use.
func TestVmBoxSource_DiskOnlyArmsUnchanged(t *testing.T) {
	def := vmBoxDef(t, "#VmBoxSource")

	for _, body := range []string{
		`{kind: "cloud_image", url: "https://example.invalid/arch.qcow2"}`,
		`{kind: "iso", url: "https://example.invalid/omarchy.iso"}`,
		`{kind: "bootc", box: "ghcr.io/opencharly/omarchy-boot:v1"}`,
		`{kind: "clone", from_vm: "base", from_snapshot: "snap1"}`,
		`{kind: "bootstrap"}`,
		`{kind: "container_disk", image: "quay.io/example/vmbox:v1"}`,
	} {
		accept(t, def, body)
	}

	// The kind space is CLOSED: a word outside it is refused, so the two new arms are
	// members of the enum rather than free-form strings.
	reject(t, def, `{kind: "rootfilesystem"}`, "an unknown source kind must be refused (the enum is closed)")
}

// TestVmBoxArtifacts_Accepted gates the layout record: a box that delivers a root drive
// AND a kernel names where each file lives in the image.
func TestVmBoxArtifacts_Accepted(t *testing.T) {
	def := vmBoxDef(t, "#VmBoxArtifacts")

	accept(t, def, `{rootfs_path: "/rootfs.ext4", kernel_path: "/vmlinux"}`)
	accept(t, def, `{rootfs_path: "/rootfs.ext4"}`)
	reject(t, def, `{rootfs_path: "/rootfs.ext4", bogus: 1}`,
		"the layout record is closed: an unknown key must be refused")
}

// TestVmBoxMetadata_MicroVMShapeAccepted gates the whole contract as an artefact would
// carry it: the shared VM metadata, the microVM source arm and the layout record.
func TestVmBoxMetadata_MicroVMShapeAccepted(t *testing.T) {
	accept(t, vmBoxDef(t, "#VmBoxMetadata"), `{
	distro: "ubuntu"
	arch: "x86_64"
	init: "systemd"
	version: "v0.2026287.1134"
	source: {
		kind: "rootfs"
		image: "ghcr.io/opencharly/fireactions-runner:v0.1"
		format: "ext4"
		disk_mib: 6144
	}
	artifacts: {rootfs_path: "/rootfs.ext4", kernel_path: "/vmlinux"}
}`)
}

// TestVmBoxMetadata_PublishedArtifactStillParses gates WIRE COMPATIBILITY: an artefact
// published before this change carries neither the new arms nor the layout record, and
// must keep parsing — which is why every added field is optional.
func TestVmBoxMetadata_PublishedArtifactStillParses(t *testing.T) {
	accept(t, vmBoxDef(t, "#VmBoxMetadata"), `{
	distro: "arch"
	arch: "x86_64"
	firmware: "uefi-insecure"
	source: {kind: "cloud_image", url: "https://example.invalid/arch.qcow2"}
}`)
}
