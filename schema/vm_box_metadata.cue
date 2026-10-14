// CUE schema for the VM-box metadata contract — the VM-specific half of the OCI-label
// metadata a VM box image carries (cutover plan §2.1: "a VM box is an OCI image whose
// labels carry the VM metadata contract and whose layers carry the disk artifact").
// A VM box image carries the STANDARD #BoxMetadata labels (the shared ai.opencharly.*
// hub) AND, layered ON TOP of them, #VmBoxMetadata — the VM-specific facts pods never
// carry (guest distro/arch, adopted account, firmware, init system, install strategy,
// disk provenance). Runtime-relevant the same way #BoxMetadata is: the box emitter
// bakes it at build time (sdk/deploykit EmitVmBox) and the source-less VM deploy
// (VmCapabilitiesFromLabels) reads it back. CUE-sourced here so the whole VM-box
// resolution reads ONE spec-owned type. Package-less; concatenated into spec.
//
// R8 (the byte-freeze gate): #VmBoxMetadata IS whole-struct-marshaled into ONE label
// (ai.opencharly.vm.box, spec.LabelVmBox) — like #CapabilityService →
// ai.opencharly.service. Because the label is NEW it has no pre-existing hand-struct
// anchor to freeze, so its json tags ARE the wire contract (required `!` → no
// omitempty, optional `?` → omitempty) — the deploykit reader round-trips this struct
// byte-for-byte through the label.
#VmBoxMetadata: {
	// distro — the guest distro id. Validated against the #DistroID vocabulary (the
	// closed id list the VM sources' `distro:` are checked against) at read time; a
	// plain string here so a future id needs no schema bump (the #DistroID def is
	// @go(-): it generates no Go type, so the field must be a string on the wire).
	distro?: string @go(Distro)
	// arch — the guest architecture (x86_64 | aarch64).
	arch?: string @go(Arch)
	// base_user — the adopted account the box is built around (the cloud_image source
	// adoption pattern: cloud-init's default user the image was prepared with).
	base_user?: string @go(BaseUser)
	// ssh_user — the ssh login user the deploy host connects as.
	ssh_user?: string @go(SSHUser)
	// firmware — the guest firmware mode the box boots under.
	firmware?: "bios" | "uefi-insecure" | "uefi-secure" @go(Firmware)
	// init — the guest init system (systemd | openrc).
	init?: string @go(Init)
	// charly_install — how the charly guest agent gets in: auto | scp | skip.
	charly_install?: string @go(CharlyInstall)
	// version — the box's CalVer (v0.<YYYYDDD>.<HHMM>).
	version?: string @go(Version)
	// source — provenance: which #VmSource kind produced the disk artifact and from
	// what. The generic BoxRef resolver (cutover task 4) routes every arm onto its
	// existing primitive from this record.
	source?: #VmBoxSource @go(Source)
	// artifacts — WHERE the files this box delivers live INSIDE the image, for a box
	// that carries more than one. A disk-only VM box needs no such record: its single
	// file's location is the shipped path convention (/disk.qcow2, or /disk/disk.img
	// for the KubeVirt containerDisk shape) and stays a constant in the reader. A
	// microVM box carries a ROOT DRIVE and a KERNEL together, so the reader must be
	// told where each one is and the emitter must be able to name more than one file.
	// What each artefact IS lives on #VmBoxSource (one fact, one place); this record
	// carries only the layout.
	artifacts?: #VmBoxArtifacts @go(Artifacts)
	// description — human-facing description of the box.
	description?: string @go(Description)
	// plan — the baked check plan (the SAME #Step the #LabeledDescription plan
	// carries): the acceptance surface `charly check live` walks after a from-box
	// vm: deploy — the VM analog of the pod box's baked plan.
	plan?: [...#Step] @go(Plan)
}

// #VmBoxSource — provenance of a VM box's artifact: the source kind that produced it
// plus the kind-specific origin reference. The kind space is the arms a VM BOX can be
// built from — (cloud_image | bootc | clone | bootstrap | iso | container_disk | rootfs
// | kernel) — so every arm that emits a box leaves a resolvable provenance record; one
// arm's fields are populated per kind. A #VmSource arm that emits NO box is deliberately
// absent: `imported` tracks an externally-managed disk for lifecycle only
// (`charly vm build` is a no-op), so it can never produce the artifact this record
// describes. The last two arms are the MICROVM pair — a root drive and the kernel booted
// with it — which the VMM attaches as two separate files (the substrate contract is
// `kind: firecracker`, spec#227).
#VmBoxSource: {
	kind!: "cloud_image" | "bootc" | "clone" | "bootstrap" | "iso" | "container_disk" | "rootfs" | "kernel" @go(Kind)
	// clone: the source entity the box's disk was cloned from.
	from_vm?: string @go(FromVm)
	// clone: the base snapshot id the clone overlay was created from.
	from_snapshot?: string @go(FromSnapshot)
	// bootc: the candy image ref the disk was installed from.
	box?: string @go(Box)
	// cloud_image | iso: the artifact url the disk was fetched from.
	url?: string @go(URL)
	// container_disk | rootfs | kernel: the OCI artifact ref the file was pulled from (or,
	// for a rootfs, the image whose tree was materialized into the ext4 file).
	image?: string @go(Image)
	// rootfs | kernel: what the delivered file IS — the fact the layout record
	// deliberately does not repeat.
	//
	// rootfs is a microVM ROOT DRIVE: the file the VMM attaches directly. It is built
	// from an OCI image's tree (`mke2fs -t ext4 -d <tree> <file>`), the one producer
	// that works on a host with no loop device, no device-mapper and no privileged
	// container (umbrella ledger probe P3).
	// kernel is the uncompressed vmlinux the VMM loads alongside it.
	format?: "ext4" | "raw" | "vmlinux" @go(Format)
	// rootfs: the size the rootfs was built at, in MiB. The deploy needs it to decide
	// whether the delivered filesystem has to GROW before the guest boots — a truncated
	// file alone does not grow the filesystem inside it.
	disk_mib?: int & >=1 @go(DiskMiB,type=int)
	// kernel: the digest of the delivered kernel, so a boot can name exactly the kernel
	// it ran without re-reading the source artefact.
	sha256?: string @go(SHA256)
	// kernel: the kernel RELEASE the artefact carries (the version the guest reports),
	// distinct from the artefact's own version.
	release?: string @go(Release)
	// kernel: the command line to boot it with. For a TAP network this is where the
	// guest's address is delivered (`ip=<guest>::<host>:<mask>::<iface>:off`), which is
	// why it belongs to the artefact rather than to each deploy that boots it.
	cmdline?: string @go(Cmdline)
}

// #VmBoxArtifacts — the IN-IMAGE layout of a box that delivers more than one file.
// Absent for the disk-only shapes. Present for the microVM shape, which delivers a root
// drive AND a kernel and hands the VMM both paths.
//
// Every field is optional: an image published before this record existed carries none,
// and a future shape may deliver a different subset. The paths are container-absolute
// and are validated at the emitter's boundary (sdk/deploykit.validInImagePath) — they
// are written UNQUOTED into the generated Containerfile, so an unvalidated value would
// be a build-instruction injection surface.
#VmBoxArtifacts: {
	// rootfs_path — where the root drive file lives in the image.
	rootfs_path?: string @go(RootfsPath)
	// kernel_path — where the kernel file lives in the image.
	kernel_path?: string @go(KernelPath)
}
