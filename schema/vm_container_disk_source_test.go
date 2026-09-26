package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// The `container_disk` arm exists so charly can PULL a bootable guest disk out of an
// OCI artifact and boot it through the unchanged libvirt/qemu path. The motivating
// corpus is a Cua Fleet image (a KubeVirt containerDisk: an OCI image whose layer
// holds the disk at /disk/disk.img) — the exact published artifact below was pulled,
// extracted and booted end to end during spike S1/S3.
func TestVmSourceAcceptsTheContainerDiskArm(t *testing.T) {
	err := vmSource(t, `{
		kind:   "container_disk"
		image:  "public.ecr.aws/k5j5w0x5/cua-omarchy-workspace@sha256:dd09f70f8cd35eca7871dff000538302baad63688a3008803f2238492802d743"
		distro: "omarchy"
	}`)
	if err != nil {
		t.Errorf("#VmSource rejects a real, pulled containerDisk source; a Cua Fleet "+
			"image cannot be expressed at all\n%v", err)
	}
}

// disk_path_in_image is the charly-side equivalent of KubeVirt's custom `path:` on the
// VMI — the escape hatch for a foreign artifact whose disk is not at the default
// /disk/disk.img. It must be accepted.
func TestVmSourceAcceptsCustomDiskPathInImage(t *testing.T) {
	err := vmSource(t, `{
		kind:               "container_disk"
		image:              "registry.example/fleet/workspace@sha256:0000000000000000000000000000000000000000000000000000000000000000"
		disk_path_in_image: "/custom-disk-path/fedora25.qcow2"
	}`)
	if err != nil {
		t.Errorf("#VmSource rejects a containerDisk source with a custom in-image disk path: %v", err)
	}
}

// image is the arm's whole reason to exist and is required. An empty ref must be a
// schema conflict rather than a build that fails later, at pull time, with nothing to pull.
//
// A required-but-absent field is INCOMPLETE, not conflicting, so — exactly as the iso
// arm's distro test documents — it surfaces under a CONCRETENESS check, which is what the
// host's decode performs. The empty-string and cross-branch cases surface under the
// ordinary conflict check, so both modes are asserted.
func TestVmSourceContainerDiskRequiresImage(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#VmSource"))

	concrete := func(lit string) error {
		return def.Unify(ctx.CompileString(lit)).Validate(cue.Concrete(true))
	}

	for _, lit := range []string{
		`{kind: "container_disk"}`,
		`{kind: "container_disk", image: ""}`,
		`{kind: "container_disk", url: "https://example.com/disk.qcow2"}`,
	} {
		if err := concrete(lit); err == nil {
			t.Errorf("#VmSource accepts %s, which has no OCI image ref for the arm to pull", lit)
		}
	}

	// And a complete source passes the same concreteness check, so the assertion above is
	// about the absent ref, not about the arm being broken.
	if err := concrete(`{kind: "container_disk", image: "registry.example/x@sha256:0000000000000000000000000000000000000000000000000000000000000000"}`); err != nil {
		t.Errorf("#VmSource rejects a complete container_disk source: %v", err)
	}
}

// The arm must stay discriminated: each field below belongs to a DIFFERENT source kind,
// and accepting one would silently produce a VM built by neither path — the same
// cross-branch rule the iso and bootstrap arms state.
func TestVmSourceContainerDiskRejectsCrossBranchFields(t *testing.T) {
	for _, bogus := range []struct{ name, field string }{
		{"url", `url: "https://cloud.example/disk.qcow2"`},
		{"checksum", `checksum: {type: "sha256"}`},
		{"box", `box: "omarchy"`},
		{"transport", `transport: "registry"`},
		{"rootfs", `rootfs: "btrfs"`},
		{"root_size", `root_size: "40G"`},
		{"from_vm", `from_vm: "base-vm"`},
		{"from_snapshot", `from_snapshot: "golden"`},
		{"libvirt_name", `libvirt_name: "some-domain"`},
		{"disk_path", `disk_path: "/var/lib/libvirt/x.qcow2"`},
		{"disk_format", `disk_format: "qcow2"`},
		{"builder", `builder: "pacstrap"`},
		{"builder_image", `builder_image: "omarchy-pacstrap-builder"`},
		{"base_user", `base_user: "arch"`},
	} {
		err := vmSource(t, `{
			kind:  "container_disk"
			image: "registry.example/x@sha256:0000000000000000000000000000000000000000000000000000000000000000"
			`+bogus.field+`
		}`)
		if err == nil {
			t.Errorf("#VmSource.container_disk accepts %s, which belongs to another "+
				"source kind and would be silently ignored by the container_disk build path", bogus.name)
		}
	}
}

// distro is CUE-OPTIONAL on this arm (like cloud_image, not like iso/bootstrap): the
// disk already exists inside the artifact, so there is no answer-file format that
// hard-fails without it. Presence is enforced only where it genuinely matters by the vm
// kind's own OpValidate; but a MISSPELLED id must still be a schema conflict, because it
// keys the guest package-manager selection and the five-id candy vocabulary — the silent
// zero-package trap the cloud_image arm documents.
func TestVmSourceContainerDiskRejectsUnknownDistro(t *testing.T) {
	for _, bogus := range []string{"omarchi", "Omarchy", "notadistro"} {
		err := vmSource(t, `{
			kind:   "container_disk"
			image:  "registry.example/x@sha256:0000000000000000000000000000000000000000000000000000000000000000"
			distro: "`+bogus+`"
		}`)
		if err == nil {
			t.Errorf("#VmSource.container_disk accepts distro %q, which is not in the closed "+
				"#DistroID vocabulary", bogus)
		}
	}
}

// #VmBoxSource.kind must carry container_disk: a VM box emitted from a containerDisk
// source records its provenance, and the emitter's completeness gate would otherwise
// reject a disk that was legitimately pulled from an OCI artifact.
func TestVmBoxSourceAcceptsContainerDiskProvenance(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#VmBoxSource"))
	if !def.Exists() {
		t.Fatal("#VmBoxSource is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(`{
		kind:  "container_disk"
		image: "public.ecr.aws/k5j5w0x5/cua-omarchy-workspace@sha256:dd09f70f8cd35eca7871dff000538302baad63688a3008803f2238492802d743"
	}`))
	if unified.Err() != nil {
		t.Fatalf("#VmBoxSource rejects container_disk provenance: %v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(false)); err != nil {
		t.Errorf("#VmBoxSource rejects container_disk provenance: %v", err)
	}
}
