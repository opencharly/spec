// CUE schema for the `firecracker` kind — a Firecracker microVM on a KVM host.
// #Firecracker validates ONE kind:firecracker definition: the HOST surface (how
// this KVM host runs microVMs: the VMM binary, the guest kernel, the network path,
// containerd, and optionally a fireactions runner pool) plus the microVM INSTANCE
// surface (the rootfs artefact, the machine shape, MMDS/cloud-init, vsock, console,
// snapshots).
//
// The kind is BOTH a standalone TEMPLATE (→ the typed uf.Firecracker map) and a
// deploy (→ uf.Deploy) — the EDGE-INHERIT shape every substrate shares (see
// node.cue's #FirecrackerValue). Its plan walks INSIDE the guest over SSH (the same
// venue as vm and kubevirt), so the shared #Step + #VmCloudInit defs are reused
// verbatim (R3).
//
// VENUE: `ssh`, and NO exclusive_venue. A Firecracker microVM is reached over SSH
// but takes no host-global libvirt domain and no host arbiter lease; claiming an
// exclusive venue would make IsVmVenue (ssh ∧ exclusive_venue) true and misroute the
// deploy into `charly vm build`/`vm create` — the exact bug spec's fe624e5 fixed for
// kubevirt. A firecracker deploy adds NO new venue value.
//
// CLOSED: an unknown key is a typo (the same strictness every kind def carries).
// Cross-rules CUE cannot express without degrading gengotypes are enforced by the
// plugin's deep OpValidate, the split #KubeVirt and #Android document:
//   * exactly one of kernel.from / kernel.path;
//   * a deploy needs image + kernel (a host-only template may omit image);
//   * network.mode cni requires network.cni; mode tap requires network.tap;
//   * containerd.snapshotter selects which of the snapshotter blocks is legal, and
//     devmapper requires its device;
//   * jailer.enabled requires jailer uid_base/gid_base;
//   * a pool needs at least one label, and its runner image is required.

#Firecracker: {
	description?: string
	plan?: [...#Step]

	// --- the HOST side: how this KVM host runs microVMs -------------------------
	// vmm pins the Firecracker release installed on the host; absent → the host's
	// own `firecracker` from PATH (the upstream default).
	vmm?:         #FirecrackerVMM          @go(VMM,optional=nillable)
	kernel:       #FirecrackerKernel       @go(Kernel)
	network?:     #FirecrackerNetwork      @go(Network,optional=nillable)
	containerd?:  #FirecrackerContainerd   @go(Containerd,optional=nillable)
	fireactions?: #FirecrackerFireactions  @go(Fireactions,optional=nillable)

	// --- the microVM INSTANCE --------------------------------------------------
	image:    #FirecrackerImage    @go(Image)
	machine?: #FirecrackerMachine  @go(Machine,optional=nillable)
	mmds?:    #FirecrackerMMDS     @go(MMDS,optional=nillable)
	cloud_init?: #VmCloudInit      @go(CloudInit,optional=nillable)
	vsock?:   #FirecrackerVsock    @go(Vsock,optional=nillable)
	console?: #FirecrackerConsole  @go(Console,optional=nillable)
	snapshot?: #FirecrackerSnapshot @go(Snapshot,optional=nillable)
}

// #FirecrackerVMM pins the VMM release and its integrity, and selects the jailer
// posture. The jailer is OFF by default: it is Firecracker's production hardening
// posture (a privileged jailer drops privileges and execs the VMM), and the
// integration's upstream orchestrator runs the VMM as a plain process.
#FirecrackerVMM: {
	version: string & !="" // e.g. "v1.17.0"
	sha256?: string        // the release tarball digest, verified before install
	binary_path?: string   // where the binary lands on the host
	jailer?: #FirecrackerJailer @go(Jailer,optional=nillable)
}

#FirecrackerJailer: {
	enabled: *false | true @go(Enabled,type=bool)
	// One jail per microVM needs its OWN uid and gid; uid_base/gid_base are the
	// allocation floor the substrate increments per VM (required when enabled).
	uid_base?: int & >=1 @go(UIDBase,type=int)
	gid_base?: int & >=1 @go(GIDBase,type=int)
	chroot_base?: string
	cgroup_version?: *"2" | "1" @go(CgroupVersion)
	// resource_limits are the jailer's own rlimits, `<resource>=<value>` verbatim
	// (fsize, no-file).
	resource_limits?: [...string] @go(ResourceLimits)
}

// #FirecrackerKernel is the guest kernel the microVMs boot. `from` is an artefact
// reference (an OCI artefact whose layer carries the kernel, or a pinned URL);
// `path` is a file already present on the host. Exactly one is required.
#FirecrackerKernel: {
	from?: string & !=""  // artefact ref / URL
	path?: string & !=""  // host path
	sha256?: string       // integrity for either form
	// cmdline — the kernel command line. The guest's IP, when the network is a
	// TAP, arrives HERE (`ip=<guest>::<host>:<mask>::<iface>:off`), the same
	// channel the CNI path uses; the substrate defaults it when absent.
	cmdline?: string
	initrd?: string
}

// #FirecrackerNetwork is how guests reach the network. `tap` (the default) is one
// TAP device per microVM with the guest's address delivered on the kernel command
// line; `cni` is the netns+bridge+tc-redirect-tap path the upstream orchestrator
// uses, which needs CAP_SYS_ADMIN.
#FirecrackerNetwork: {
	mode: *"tap" | "cni"
	tap?: #FirecrackerTAP @go(Tap,optional=nillable)
	cni?: #FirecrackerCNI @go(CNI,optional=nillable)
	// egress_iface is the host uplink to masquerade through; "auto" = the
	// default-route interface.
	egress_iface?: *"auto" | string @go(EgressIface)
	forwarding?: #FirecrackerForwarding @go(Forwarding,optional=nillable)
	// resolver — the guest's /etc/resolv.conf servers. A stock Firecracker guest
	// image ships none, so IP egress can work while DNS does not; naming them here
	// is what makes the difference checkable.
	resolver?: [...string] @go(Resolver)
	mtu?: int & >=68 & <=65535 @go(MTU,type=int)
}

#FirecrackerTAP: {
	// name is a template; `{n}` is replaced by the per-lease index.
	name?: string
	// subnet + prefix allocate one /prefix per microVM (host_offset is the host's
	// own address inside it).
	subnet?: string
	prefix?: *30 | int & >=8 & <=30 @go(Prefix,type=int)
	host_offset?: *1 | int & >=1 @go(HostOffset,type=int)
}

#FirecrackerCNI: {
	network_name?: string
	conf_dir?: string
	bin_dir?: string
	cache_dir?: string
	subnet?: string
}

// #FirecrackerForwarding is the host forwarding posture. `append-accept` inserts an
// ordered FORWARD accept (the host's own policy may be DROP, which a separate nft
// table cannot override); `netns` puts guest traffic in a namespace whose
// forwarding the substrate controls.
#FirecrackerForwarding: {
	ip_forward: *true | false @go(IPForward,type=bool)
	policy?: *"append-accept" | "netns" | "deny" @go(Policy)
}

// #FirecrackerMachine is the microVM shape. vcpu_count and mem_size_mib mirror
// Firecracker's own machine-config (vcpu_count is 1–32, and either 1 or even; smt is
// x86-only).
#FirecrackerMachine: {
	vcpu_count?: int & >=1 & <=32 @go(VCPUCount,type=int)
	mem_size_mib?: int & >=128 @go(MemSizeMiB,type=int)
	smt?: bool
	huge_pages?: *"None" | "Transparent" | "2M" @go(HugePages)
	// cpu_template mirrors Firecracker's (deprecated) CpuTemplate enum; it exists
	// for snapshot/clone portability across hosts.
	cpu_template?: *"None" | "C3" | "T2" | "T2S" | "T2CL" | "T2A" | "V1N1" @go(CPUTemplate)
	drives?: [...#FirecrackerDrive] @go(Drives)
	// rate_limits bound the interfaces and the root drive; an omitted bucket is
	// unlimited (Firecracker's own semantics).
	rate_limits?: #FirecrackerRateLimits @go(RateLimits,optional=nillable)
}

// #FirecrackerDrive is one virtio-block device. The root drive is built from the
// image; extra drives are named here.
#FirecrackerDrive: {
	drive_id!: string & !="" @go(DriveID)
	path?: string @go(Path)
	is_root_device?: bool @go(IsRootDevice)
	is_read_only?: bool @go(IsReadOnly)
	cache_type?: *"Unsafe" | "Writeback" @go(CacheType)
	// io_engine Async needs a host kernel newer than 5.10.51.
	io_engine?: *"Sync" | "Async" @go(IOEngine)
	blk_size?: int & >=512 @go(BlkSize,type=int)
	// discard is only honoured by a writable virtio-block device on the Sync engine.
	discard?: bool @go(Discard)
	rate_limiter?: #FirecrackerRateLimiter @go(RateLimiter,optional=nillable)
}

#FirecrackerRateLimits: {
	network_in?:  #FirecrackerRateLimiter @go(NetworkIn,optional=nillable)
	network_out?: #FirecrackerRateLimiter @go(NetworkOut,optional=nillable)
	rootfs?:      #FirecrackerRateLimiter @go(Rootfs,optional=nillable)
	serial?:      #FirecrackerRateLimiter @go(Serial,optional=nillable)
}

// #FirecrackerRateLimiter is Firecracker's token bucket: the rate is
// size/refill_time, consumption below the refill rate is unbounded (bursts).
#FirecrackerRateLimiter: {
	bandwidth?: #FirecrackerTokenBucket @go(Bandwidth,optional=nillable)
	ops?:       #FirecrackerTokenBucket @go(Ops,optional=nillable)
}

#FirecrackerTokenBucket: {
	size: int & >0 @go(,type=int)
	refill_time: int & >0 @go(RefillTime,type=int) // milliseconds
	one_time_burst?: int & >=0 @go(OneTimeBurst,type=int)
}

// #FirecrackerImage is the microVM root drive: the artefact charly BUILT (a
// kind:firecracker deploy's image is a charly-produced rootfs, not somebody else's
// container export). `format` is what the artefact's labels declare; a raw ext4
// rootfs is the file-backed form, and `resize` grows the inner filesystem to
// disk_mib on first boot (Firecracker attaches FILES — a truncated file alone does
// not grow the filesystem inside it).
#FirecrackerImage: {
	from: string & !="" @go(From)
	format?: *"ext4" | "raw" @go(Format)
	disk_mib?: int & >=128 @go(DiskMiB,type=int)
	resize?: *true | false @go(Resize,type=bool)
}

// #FirecrackerMMDS is the metadata service inside the guest. V2 is a session-token
// IMDSv2 shape; the upstream orchestrator reads exactly
// latest/meta-data/fireactions/{runner_jit_config,hostname,shutdown_on_exit} from
// it, which is why the tree is authored under `latest/meta-data` here.
#FirecrackerMMDS: {
	version?: *"V2" | "V1" @go(Version)
	ipv4_address?: string @go(IPv4Address)
	// metadata is the tree under latest/meta-data (typed-open: its shape is the
	// consumer's business — the orchestrator's, the candy's, the operator's).
	metadata?: {[string]: _} @go(Metadata)
}

#FirecrackerVsock: {
	// guest_cid must be >= 3 (2 is the host); the substrate allocates one per
	// microVM when absent, since one device serves all guests.
	guest_cid?: int & >=3 @go(GuestCID,type=int)
	uds_path?: string @go(UDS_Path)
}

#FirecrackerConsole: {
	serial_out_path?: string @go(SerialOutPath)
	rate_limiter?: #FirecrackerRateLimiter @go(RateLimiter,optional=nillable)
	log_path?: string @go(LogPath)
	log_level?: *"Info" | "Error" | "Warning" | "Debug" | "Trace" | "Off" @go(LogLevel)
	metrics_path?: string @go(MetricsPath)
}

#FirecrackerSnapshot: {
	create?: bool @go(Create)
	mem_file_path?: string @go(MemFilePath)
	snapshot_path?: string @go(SnapshotPath)
	snapshot_type?: *"Full" | "Diff" @go(SnapshotType)
	resume?: *true | false @go(Resume,type=bool)
}

// #FirecrackerContainerd is the containerd this host runs for image storage: the
// connection, and the SNAPSHOTTER that backs a microVM root drive. Firecracker
// attaches a block device or a FILE, so only the two file/device-backed
// snapshotters can serve a root drive: `devmapper` (an LVM thin pool on a dedicated
// block device — the upstream default) and `blockfile` (a file per snapshot, which
// needs neither LVM nor a spare device). A snapshotter whose mount is a directory
// (overlayfs, native) can never work.
#FirecrackerContainerd: {
	address?: string @go(Address)
	namespace?: string @go(Namespace)
	snapshotter: *"devmapper" | "blockfile" @go(Snapshotter)
	// manage says how charly treats an existing /etc/containerd/config.toml:
	// `merge` (default) keeps every setting it does not model, `full` takes the
	// file over, `adopt` changes nothing and only reports.
	manage?: *"merge" | "full" | "adopt" @go(Manage)
	version?: int & >=2 & <=3 @go(Version,type=int)
	root?: string @go(Root)
	state?: string @go(State)
	oom_score?: int @go(OOMScore,type=int)
	grpc?: #FirecrackerContainerdGRPC @go(GRPC,optional=nillable)
	devmapper?: #FirecrackerDevmapper @go(Devmapper,optional=nillable)
	// plugins is a typed-open passthrough for any containerd plugin table charly
	// does not model yet.
	plugins?: {[string]: _} @go(Plugins)
	registry_mirrors?: [...#FirecrackerRegistryMirror] @go(RegistryMirrors)
	service?: #FirecrackerContainerdService @go(Service,optional=nillable)
}

#FirecrackerContainerdGRPC: {
	address?: string @go(Address)
	uid?: int & >=0 @go(UID,type=int)
	gid?: int & >=0 @go(GID,type=int)
}

// #FirecrackerDevmapper is the devmapper snapshotter's pool. `device` is the block
// device the thin pool is created on — charly provisions it (pvcreate/vgcreate/
// lvcreate + the autoextend profile) rather than leaving it to a hand-typed
// command, and refuses with a visible skip when the host cannot satisfy it.
#FirecrackerDevmapper: {
	pool?: string @go(Pool)
	root_path?: string @go(RootPath)
	base_image_size?: string @go(BaseImageSize)
	discard_blocks?: bool @go(DiscardBlocks)
	fs_type?: *"ext4" | "xfs" @go(FSType)
	device?: string @go(Device)
	profile?: #FirecrackerThinPoolProfile @go(Profile,optional=nillable)
}

#FirecrackerThinPoolProfile: {
	thin_pool_autoextend_threshold?: int & >=1 & <=100 @go(ThinPoolAutoextendThreshold,type=int)
	thin_pool_autoextend_percent?: int & >=1 & <=100 @go(ThinPoolAutoextendPercent,type=int)
}

#FirecrackerRegistryMirror: {
	host: string & !="" @go(Host)
	endpoint?: string @go(Endpoint)
	insecure?: bool @go(Insecure)
}

#FirecrackerContainerdService: {
	// unit distro → keep the distribution's own containerd unit; charly → render
	// one (the mixed-entry service polymorphism, per distro).
	unit?: *"distro" | "charly" @go(Unit)
	limits?: {[string]: _} @go(Limits)
}

// #FirecrackerFireactions is the OPTIONAL runner-pool orchestrator on this host. Its
// pools are a policy for how microVMs are created and registered as GitHub Actions
// runners; the substrate RENDERS this into the orchestrator's own configuration
// (validated before it is written) and supervises it — it does not reimplement the
// scheduler.
#FirecrackerFireactions: {
	version?: string & !="" @go(Version)
	install?: #FirecrackerFireactionsInstall @go(Install,optional=nillable)
	// bind_address defaults to LOOPBACK: the orchestrator's API is plaintext and
	// unauthenticated upstream, so naming a wider address is a deliberate act.
	bind_address?: *"127.0.0.1:8080" | string @go(BindAddress)
	metrics?: #FirecrackerFireactionsMetrics @go(Metrics,optional=nillable)
	log_level?: *"info" | "debug" | "warn" | "error" @go(LogLevel)
	github?: #FirecrackerFireactionsGitHub @go(GitHub,optional=nillable)
	pools: [...#FirecrackerPool] @go(Pools)
}

#FirecrackerFireactionsInstall: {
	// from_source builds the pinned tag (the upstream release binaries carry no
	// version stamp without the release toolchain's ldflags).
	from_source?: *true | false @go(FromSource,type=bool)
	go?: string @go(Go)
}

#FirecrackerFireactionsMetrics: {
	enabled: *true | false @go(Enabled,type=bool)
	address?: string @go(Address)
}

// #FirecrackerFireactionsGitHub names the GitHub App credential. The private key is
// NEVER authored here: it is a secret reference resolved from the credential store
// at deploy time.
#FirecrackerFireactionsGitHub: {
	app_id?: int & >0 @go(AppID,type=int)
	app_private_key_secret?: string @go(AppPrivateKeySecret)
}

// #FirecrackerPool is one runner pool: a label set + a runner image + a machine
// shape, kept at a replica count.
#FirecrackerPool: {
	name: string & !="" @go(Name)
	replicas?: int & >=0 @go(Replicas,type=int)
	shutdown_on_exit?: *true | false @go(ShutdownOnExit,type=bool)
	runner: #FirecrackerRunner @go(Runner)
	firecracker?: #FirecrackerPoolMachine @go(Firecracker,optional=nillable)
}

#FirecrackerRunner: {
	name?: string @go(Name)
	// image is a runner guest image: charly's own (a box built into a rootfs
	// artefact) or an upstream-compatible one.
	image: string & !="" @go(Image)
	image_pull_policy?: *"IfNotPresent" | "Always" | "Never" @go(ImagePullPolicy)
	// organization is ORGANISATION-scoped: the orchestrator's GitHub calls are
	// org-level only (a repository-scoped runner is not expressible).
	organization?: string @go(Organization)
	group_id?: int & >=0 @go(GroupID,type=int)
	labels: [...string] @go(Labels)
}

#FirecrackerPoolMachine: {
	machine_config?: #FirecrackerMachine @go(MachineConfig,optional=nillable)
	metadata?: {[string]: _} @go(Metadata)
	rate_limits?: #FirecrackerRateLimits @go(RateLimits,optional=nillable)
}
