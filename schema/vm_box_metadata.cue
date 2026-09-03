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
	// description — human-facing description of the box.
	description?: string @go(Description)
	// plan — the baked check plan (the SAME #Step the #LabeledDescription plan
	// carries): the acceptance surface `charly check live` walks after a from-box
	// vm: deploy — the VM analog of the pod box's baked plan.
	plan?: [...#Step] @go(Plan)
}

// #VmBoxSource — provenance of a VM box's disk artifact: the source kind that produced
// it plus the kind-specific origin reference. The kind space mirrors #VmSource's arms
// (cloud_image | bootc | clone | bootstrap | iso) so every arm a VM was built from
// leaves a resolvable provenance record; one arm's fields are populated per kind.
#VmBoxSource: {
	kind!: "cloud_image" | "bootc" | "clone" | "bootstrap" | "iso" @go(Kind)
	// clone: the source entity the box's disk was cloned from.
	from_vm?: string @go(FromVm)
	// clone: the base snapshot id the clone overlay was created from.
	from_snapshot?: string @go(FromSnapshot)
	// bootc: the candy image ref the disk was installed from.
	box?: string @go(Box)
	// cloud_image | iso: the artifact url the disk was fetched from.
	url?: string @go(URL)
}
