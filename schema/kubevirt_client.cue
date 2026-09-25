// CUE schema for the KUBEVIRT-CLIENT wire family — the host↔plugin-kubevirt
// internal-op RPC types (NOT authored config — never in #Node/#Op), mirroring
// the vmclient.cue family (R3: the same SDD treatment for the 6th substrate).
// These are the payloads the `command:kubevirt` CLI dispatches to
// candy/plugin-kubevirt's internal ops (domain-state / list / resolve-console /
// snapshot / migrate). Package-less; concatenated into the spec compilation
// unit.
//
// Written out explicitly rather than embedding #KubeVirt — the wire env carries
// PLAIN post-default scalars, not #KubeVirt's own enum/default machinery.

// #KubeVirtPluginEnv is the host→plugin env for an internal KubeVirt-resolution
// RPC. vm_op selects the operation: list / vm-state / resolve-vnc / resolve-console
// / snapshot-internal / migrate.
#KubeVirtPluginEnv: {
	vm_op!:   string @go(VmOp)
	name?:    string @go(Name)
	namespace?: string @go(Namespace)
	cluster?: string @go(Cluster)
	kube_context?: string @go(KubeContext)
	kubeconfig?:   string @go(Kubeconfig)
	force?:   bool @go(Force)
	delete_volumes?: bool @go(DeleteVolumes)
	snap?:    #KubeVirtSnapInternalReq @go(Snap,optional=nillable)
	migrate?: #KubeVirtMigrationReq     @go(Migrate,optional=nillable)
}

// #KubeVirtSnapshotCreateOpts parameterizes a VirtualMachineSnapshot create.
#KubeVirtSnapshotCreateOpts: {
	vm_name!:       string @go(VmName)
	snap_name!:     string @go(SnapName)
	namespace?:     string @go(Namespace)
	snapshot_class?: string @go(SnapshotClass)
	description?:   string @go(Description)
}

// #KubeVirtSnapshotEntry is one snapshot record (the on-disk registry shape) —
// carried on delete/revert ops so the plugin has the full record.
#KubeVirtSnapshotEntry: {
	name!:        string @go(Name)
	namespace?:   string @go(Namespace)
	snapshot_class?: string @go(SnapshotClass)
	description?: string @go(Description)
	created?:     string @go(Created)
	phase?:       string @go(Phase)
	refcount!:    int    @go(Refcount,type=int)
}

// #KubeVirtSnapInternalReq is the snapshot-internal op payload.
#KubeVirtSnapInternalReq: {
	snap_op!:  string                   @go(SnapOp)
	vm_name!:  string                   @go(VmName)
	opts?:     #KubeVirtSnapshotCreateOpts @go(Opts,optional=nillable)
	entry?:    #KubeVirtSnapshotEntry      @go(Entry,optional=nillable)
}

// #KubeVirtMigrationReq parameterizes a VirtualMachineInstanceMigration.
#KubeVirtMigrationReq: {
	vm_name!:  string @go(VmName)
	namespace?: string @go(Namespace)
	to_node?:  string @go(ToNode)
}

// #KubeVirtDomainInfo mirrors a live KubeVirt VM row for status collection.
#KubeVirtDomainInfo: {
	name:  string @go(Name)
	state: string @go(State)
}

// #KubeVirtConsoleEndpoint describes how to reach a running VM's console/VNC.
#KubeVirtConsoleEndpoint: {
	kind?:        string @go(Kind)
	namespace?:   string @go(Namespace)
	name?:        string @go(Name)
	host?:        string @go(Host)
	port?:        int    @go(Port,type=int)
	tunnel_needed?: bool @go(TunnelNeeded)
}

// #KubeVirtResolveResult decodes a resolve-console reply.
#KubeVirtResolveResult: {
	endpoint?: #KubeVirtConsoleEndpoint @go(Endpoint)
	error?:    string                     @go(Error)
}
