// CUE schema for the `kubevirt` kind — the 6th deploy substrate. #KubeVirt validates
// ONE kind:kubevirt VM definition: a KubeVirt VirtualMachine on a Kubernetes cluster.
// The kind is BOTH a standalone TEMPLATE (→ the typed uf.Kubevirt map) and a deploy
// (→ uf.Deploy) — the EDGE-INHERIT shape every substrate shares (see node.cue's
// #KubevirtValue). Its plan walks INSIDE the guest over SSH (the same venue as vm),
// so the shared #Step + #VmCloudInit defs are reused verbatim (R3).
//
// CLOSED: an unknown key is a typo. The Go type is spec.KubeVirt; the flat
// discriminated-union boot medium is hand-written as spec.KubevirtSource in
// spec/union_types.go (the #VmSource precedent — a CUE union degrades under
// gengotypes, so the union itself is @go(-) and the Go shape is hand-built).
//
// Cross-rules that CUE cannot express cleanly (exactly-one boot arm, GPU
// resource_name XOR device_name, instancetype XOR explicit cpu/memory) are enforced
// by the kind:kubevirt provider's deep OpValidate (candy/plugin-kubevirt) — the
// same "CUE closes the field set, Go restores the XOR" split #Android documents.

#KubeVirt: {
	// cluster — a kind:kubernetes cluster template name (the SAME cluster model the
	// Kubernetes deploy substrate uses). kube_context pins a concrete kubeconfig
	// context directly, bypassing the template lookup.
	cluster?:     string & !=""
	kube_context?: string @go(KubeContext)
	namespace?:   string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	storage_class?: string @go(StorageClass)
	description?:   string
	plan?: [...#Step]

	// source — the boot medium. Exactly one arm (enforced in OpValidate).
	source: #KubevirtSource @go(Source,type=KubevirtSource)

	// --- domain shape ---
	memory?: #VmSize @go(Memory)
	cpu?:    #KubevirtCPU @go(CPU,optional=nillable)
	machine?: "q35" | "pc-i440fx" | "" @go(Machine)
	firmware?: #KubevirtFirmware @go(Firmware,optional=nillable)
	devices?:  #KubevirtDevices  @go(Devices,optional=nillable)

	// gpus — KubeVirt host-device passthrough. Each entry names EITHER a cluster
	// device-plugin resource (resource_name: "nvidia.com/gpu") XOR a specific host
	// device (device_name), enforced in OpValidate. Maps requires_exclusive: to
	// KubeVirt's spec.domain.devices.gpus — the cluster device plugin owns the
	// physical card (no host arbiter involvement, unlike the vm substrate).
	gpus?: [...#KubevirtGPU] @go(GPUs)

	cloud_init?: #VmCloudInit @go(CloudInit,optional=nillable)
	network?:    #KubevirtNetwork @go(Network,optional=nillable)

	instancetype?: string
	preference?:   string

	run_strategy?:      *"Always" | "Halted" | "Manual" | "RerunOnFailure" | "Once" @go(RunStrategy)
	eviction_strategy?: "None" | "LiveMigrate" | "LiveMigrateIfPossible" | "External"  @go(EvictionStrategy)
	termination_grace_period_seconds?: int & >=0 @go(TerminationGracePeriodSeconds,type=int)

	node_selector?: {[string]: string} @go(NodeSelector)
	// affinity — a raw Kubernetes Affinity object; genuine passthrough.
	affinity?: {...}
	migration?: #KubevirtMigration @go(Migration,optional=nillable)
	snapshot?:  [...#KubevirtSnapshot] @go(Snapshots)
}

// 5-way discriminated union on source.kind; each arm pins kind, requires its
// fields, forbids cross-branch fields via _|_, and is CLOSED.
#KubevirtSource:
	{
		kind: "container_disk"
		// image is an OCI image whose layers carry the disk — a charly VM box
		// (its ai.opencharly.* labels carry the VM metadata contract) or any
		// KubeVirt containerDisk image.
		image: string & !=""
		pull_policy?: "Always" | "IfNotPresent" | "Never" @go(PullPolicy)
		pull_secret?: string @go(PullSecret)
		data_volume?: _|_
		pvc?:         _|_
		clone?:       _|_
		size?:        _|_
	} | {
		kind: "data_volume"
		// source — the CDI import source (http/registry/pvc/blank). Typed-open:
		// the CDI DataVolume source vocabulary is CDI's business.
		data_volume: {[string]: _}
		size: string & !=""
		storage_class?: string @go(StorageClass)
		content_type?: "kubevirt" | "archive" @go(ContentType)
		image?: _|_
		pull_policy?: _|_
		pull_secret?: _|_
		pvc?: _|_
		clone?: _|_
	} | {
		kind: "pvc"
		pvc: string & !=""
		image?: _|_
		pull_policy?: _|_
		pull_secret?: _|_
		data_volume?: _|_
		size?: _|_
		clone?: _|_
	} | {
		kind: "clone"
		// clone — create a DataVolume as a clone of an existing VM/DataVolume.
		clone: { from: string & !="" }
		size?: string
		storage_class?: string @go(StorageClass)
		image?: _|_
		pull_policy?: _|_
		pull_secret?: _|_
		data_volume?: _|_
		pvc?: _|_
	} @go(-) // gengotypes: hand KubevirtSource (spec/union_types.go) — flat discriminated struct

#KubevirtCPU: {
	cores?:   int & >=1 @go(,type=int)
	sockets?: int & >=1 @go(,type=int)
	threads?: int & >=1 @go(,type=int)
	model?:   string @go(Model)
	dedicated_cpu_placement?: bool @go(DedicatedCPUPlacement)
}

#KubevirtFirmware: {
	// bootloader selects BIOS or UEFI. Required-with-default so the efi-secure
	// cross-rule below can reference it (an optional field errors when absent).
	bootloader: *"bios" | "efi" @go(Bootloader)
	efi_secure_boot?: bool @go(EFISecureBoot)
	// Secure Boot needs UEFI.
	if efi_secure_boot == true {
		bootloader: "efi"
	}
}

#KubevirtDevices: {
	// autoattach_* mirror the KubeVirt autoattach toggles.
	autoattach_pod_interface?: bool @go(AutoattachPodInterface)
	autoattach_graphics_device?: bool @go(AutoattachGraphicsDevice)
	autoattach_serial_console?: bool @go(AutoattachSerialConsole)
	// rng adds a virtio-rng device (entropy for a cloud-init guest).
	rng?: bool
	// disks/filesystems are the extra (non-boot) volumes, typed-open passthrough.
	disks?: [...{[string]: _}]
	inputs?: [...#KubevirtInput]
	watchdog?: #KubevirtWatchdog @go(Watchdog,optional=nillable)
}

#KubevirtInput: {
	type: "tablet" | "mouse" | "keyboard"
	bus?: "virtio" | "usb" | "ps2"
}

#KubevirtWatchdog: {
	model: "i6300esb" | "diag288"
	action?: "poweroff" | "reset" | "shutdown"
}

#KubevirtGPU: {
	resource_name?: string & !="" @go(ResourceName)
	device_name?:   string & !="" @go(DeviceName)
}

#KubevirtNetwork: {
	// pod is the default KubeVirt network (a masquerade interface over the pod net).
	interface?: *"masquerade" | "bridge" | "pod" @go(Interface)
	model?:     *"virtio" | "e1000" | "rtl8139" @go(Model)
	// ports declare named ports on the interface (informational for a Service).
	ports?: [...#KubevirtPort]
}

#KubevirtPort: {
	name?: string
	port:  int & >=1 & <=65535 @go(,type=int)
	protocol?: *"TCP" | "UDP" @go(Protocol)
}

#KubevirtMigration: {
	allow_auto_converge?: bool   @go(AllowAutoConverge)
	allow_post_copy?:     bool   @go(AllowPostCopy)
	bandwidth_per_migration?: string @go(BandwidthPerMigration)
	completion_timeout_seconds?: int @go(CompletionTimeoutSeconds,type=int)
}

#KubevirtSnapshot: {
	name: string & !=""
	description?: string
	// snapshot_class names the VolumeSnapshotClass; empty → the cluster default.
	snapshot_class?: string @go(SnapshotClass)
}
