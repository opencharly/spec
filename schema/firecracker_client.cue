// CUE schema for the FIRECRACKER-CLIENT wire family — the host↔plugin-firecracker
// internal-op RPC types (NOT authored config — never in #Node/#Op), mirroring the
// kubevirt_client.cue and vmclient.cue families (R3: the same SDD treatment for the
// 8th substrate). These are the payloads the `command:firecracker` CLI dispatches to
// candy/plugin-firecracker's internal ops (machine-state / list / resolve-console /
// mmds / snapshot / pools / doctor). Package-less; concatenated into the spec
// compilation unit.
//
// Written out explicitly rather than embedding #Firecracker — the wire env carries
// PLAIN post-default scalars, not #Firecracker's own enum/default machinery.

// #FirecrackerPluginEnv is the host→plugin env for an internal Firecracker-resolution
// RPC. machine_op selects the operation: list / machine-state / resolve-console /
// mmds-internal / snapshot-internal / pool-state / doctor.
#FirecrackerPluginEnv: {
	machine_op!: string @go(MachineOp)
	name?:   string @go(Name)
	host?:   string @go(Host)
	ssh_user?: string @go(SSHUser)
	force?:  bool @go(Force)
	delete_disks?: bool @go(DeleteDisks)
	mmds?: #FirecrackerMMDSReq @go(MMDS,optional=nillable)
	snap?: #FirecrackerSnapInternalReq @go(Snap,optional=nillable)
}

// #FirecrackerMachineInfo mirrors a live microVM row for status collection.
#FirecrackerMachineInfo: {
	name:          string @go(Name)
	state:         string @go(State)
	guest_ip?:     string @go(GuestIP)
	vcpu_count?:   int    @go(VCPUCount,type=int)
	mem_size_mib?: int    @go(MemSizeMiB,type=int)
	pool?:         string @go(Pool)
}

// #FirecrackerPoolInfo mirrors one orchestrator pool row.
#FirecrackerPoolInfo: {
	name:             string @go(Name)
	organization?:    string @go(Organization)
	replicas?:        int    @go(Replicas,type=int)
	current_replicas?: int   @go(CurrentReplicas,type=int)
	state?:           string @go(State)
	image?:           string @go(Image)
}

// #FirecrackerConsoleEndpoint describes how to reach a running microVM's console.
#FirecrackerConsoleEndpoint: {
	// kind selects the channel: "serial" (the Firecracker console log), "ssh" (the
	// guest over the TAP/CNI address) or "vsock" (the guest agent socket).
	kind?: string @go(Kind)
	host?: string @go(Host)
	port?: int    @go(Port,type=int)
	path?: string @go(Path)
	user?: string @go(User)
}

// #FirecrackerMMDSReq is an MMDS read/write payload (put replaces the tree, patch is
// a JSON merge patch).
#FirecrackerMMDSReq: {
	mmds_op!: string @go(MMDSOp)
	path?:     string @go(Path)
	document?: bytes  @go(Document,type=RawBody)
}

// #FirecrackerSnapInternalReq is the snapshot-internal op payload.
#FirecrackerSnapInternalReq: {
	snap_op!:      string @go(SnapOp)
	name!:         string @go(Name)
	mem_file_path?: string @go(MemFilePath)
	snapshot_path?: string @go(SnapshotPath)
	snapshot_type?: string @go(SnapshotType)
	resume?:       bool   @go(Resume)
	entry?: #FirecrackerSnapshotEntry @go(Entry,optional=nillable)
}

// #FirecrackerSnapshotEntry is one snapshot record (the on-disk registry shape) —
// carried on delete/revert ops so the plugin has the full record.
#FirecrackerSnapshotEntry: {
	name?:         string @go(Name)
	mem_file_path?: string @go(MemFilePath)
	snapshot_path?: string @go(SnapshotPath)
	snapshot_type?: string @go(SnapshotType)
	created?:      string @go(Created)
	refcount!:     int    @go(Refcount,type=int)
}

// #FirecrackerDoctorReport is the host-precondition probe result. Read-only: it
// reports what the host can do (KVM, the binary, the kernel, the rootfs, whether the
// configured snapshotter is actually served, and the forwarding posture) so a
// precondition failure is a report, never a guess.
#FirecrackerDoctorReport: {
	kvm?:                 bool   @go(KVM)
	kvm_path?:            string @go(KVMPath)
	binary?:              string @go(Binary)
	binary_version?:      string @go(BinaryVersion)
	kernel?:              string @go(Kernel)
	rootfs?:              string @go(Rootfs)
	snapshotter?:         string @go(Snapshotter)
	snapshotter_served?:  bool   @go(SnapshotterServed)
	network_mode?:        string @go(NetworkMode)
	forward_policy?:      string @go(ForwardPolicy)
	problems?:            [...string] @go(Problems)
}

// #FirecrackerResolveResult decodes a resolve-console reply.
#FirecrackerResolveResult: {
	endpoint?: #FirecrackerConsoleEndpoint @go(Endpoint)
	error?:    string                       @go(Error)
}
