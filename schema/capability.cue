package schema

import "list"

// capability.cue — the authored capability-ask surface (§8, §9), PORTED from DESIGN.cue, not
// re-designed.
//
// DESIGN.cue is the single schema copy (PROGRAM/design-implementation.md), and it already declares
// this contract in full: the M4 host words at DESIGN.cue:217-244, the §8 capability table and its
// shapes at DESIGN.cue:466-537, the M3 driver table its `driver_api` row draws its words from at
// DESIGN.cue:245-274, and the role vocabulary the table's last row loops over at DESIGN.cue:1138.
// This file is that declaration moved into the contract module so Go can be generated from it — the
// NAMES, the WORD TABLES and the FIELD SETS are DESIGN.cue's, verbatim. Nothing here is a new shape.
//
// The primitives it names are DESIGN.cue's too (DESIGN.cue:49-100), byte for byte, because a port
// that re-spells a primitive is not a port; the ones the module already declares are REUSED, never
// re-declared: #Word and #Ref (audit.cue), #Text (stage.cue), #Count (stage.cue), #Size
// (00_common.cue), #AbsPath and #SecretUse (secret.cue).
//
// THREE NAMES WERE ONCE SHARED, and the finding is now CLOSED. Recorded because the failure mode is
// invisible and will recur: three of DESIGN.cue's generic mechanism words had already been spent by
// other schema files on unrelated concepts -- `_device`/`_driver` by schema/vm.cue (libvirt VIDEO
// devices and FILESYSTEM drivers), `_access` by schema/deploy.cue (VOLUME access modes, a vocabulary
// DESIGN.cue does not have at all). CUE unifies two definitions of one name, so each pair merged
// SILENTLY: the union of the two `_device` tables evaluated to 14 words with exit 0, and neither
// `cue-gen` nor `go build` refused it. Only a test that pins the EXACT word set catches it, which is
// what schema/capability_port_test.go does.
//
// RESOLVED by giving each concept its own name in its own file -- spec#354 (`_access` ->
// `_volumeAccessMode`) and spec#356 (`_device`/`_driver` -> `_libvirtVideoDevice`/
// `_libvirtFilesystemDriver`). The word tables below are therefore DESIGN's own, exactly, and the
// vocabularies are proven separate rather than merely believed separate. The general question this
// raised -- whether the S3 shape gate and DESIGN.cue can even meet -- is opencharly/spec#355.
// ── the primitives this contract names, PORTED from DESIGN.cue:49-100 ──
#Name:        =~"^[a-z][a-z0-9]*(-[a-z0-9]+)*$" // D-NEST-4: a user name, never `--`
#EnvName:     =~"^[A-Za-z_][A-Za-z0-9_]*$"
#Port:        int & >0 & <65536
#Hostname:    =~"^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$"
#InitNeutral: string & !~"%[a-zA-Z(]" // D-CANDY-3
#TextSet:     [...#Text] & list.UniqueItems()
#NativeRow:   {native?: #Text}

// ── M4: the host words a need may name (§9) — DESIGN.cue:217-244 ──
//
// DESIGN.cue marks its own tables @vocab(core)/@vocab(registry); the shipped contract marks every
// word table @vocab(spec), which is what schemagen reads, so the value is `spec` here for all of them.
_gpu: {nvidia: {}, amd: {}, intel: {}} @vocab(spec) // `gpu: {}` names no vendor: any GPU (D-HW-9)
_gpuWord: [for w, _ in _gpu {w}]
// ONE declaration, carrying both the row pattern and the rows: the S3 gate reads a table as one
// struct literal ("a table is one home", D-PAT-2), and a pattern inside a struct literal is not a
// second home — it is what makes every row a `#NativeRow`. `cue` does not iterate the pattern when
// it derives `_deviceWord`, so the list is DESIGN's eight words.
_device: {
	[#Word]: #NativeRow
	kvm:       {native: "/dev/kvm"}
	render:    {native: "/dev/dri"}
	fuse:      {native: "/dev/fuse"}
	tun:       {native: "/dev/net/tun"}
	vhost_net: {native: "/dev/vhost-net"}
	vsock:     {native: "/dev/vsock"}
	hwrng:     {native: "/dev/hwrng"}
	kfd:       {native: "/dev/kfd"}
} @vocab(spec)
_deviceWord: [for w, _ in _device {w}]
_nesting: {container: {}, vm: {}, userns: {}} @vocab(spec)
_nestingWord: [for w, _ in _nesting {w}]
_isolation: {microvm: {}} @vocab(spec)
_isolationWord: [for w, _ in _isolation {w}]
_privilege: {rootful: {}} @vocab(spec)
_privilegeWord: [for w, _ in _privilege {w}]
_memory: {ram: {}, shm: {}} @vocab(spec)
_memoryWord: [for w, _ in _memory {w}]
_cpu: {core: {}} @vocab(spec)
_cpuWord: [for w, _ in _cpu {w}]
_storage: {root: {}} @vocab(spec)
_storageWord: [for w, _ in _storage {w}]

// ── M3: the driver table (D-DRV-1, D-DRV-3) — DESIGN.cue:113-146 and :245-274 ──
//
// `_capability.driver_api` keys off `_driverWord`, so the table it derives from is part of this
// port even though the brief names only the word list: a word list is DESIGN's to derive, and a
// literal copy of it would be exactly the re-spelling this file exists to avoid.
_form: {image: {}, disk: {}} @vocab(spec)
_formWord: [for w, _ in _form {w}]
_capture: {filesystem: {}, process: {}} @vocab(spec)
_captureWord: [for w, _ in _capture {w}]
_leaseMode: {shared: {}, exclusive: {}} @vocab(spec)
_leaseModeWord: [for w, _ in _leaseMode {w}]
_transport: {tcp: {}, udp: {}} @vocab(spec)
_transportWord: [for w, _ in _transport {w}]
_protocol: {http: {}, mcp: {}, grpc: {}} @vocab(spec)
_protocolWord: [for w, _ in _protocol {w}]
_access: {exec: {}, copy: {}, log: {}, console: {}, display: {}, forward: {}, power: {}} @vocab(spec)
_accessWord: [for w, _ in _access {w}]
// Every pluggable subsystem whose driver the user configuration chooses (D-DRV-1).
_subsystem: {pod: {}, vm: {}, kubernetes: {}, agent: {}, secret: {}} @vocab(spec)
_subsystemWord: [for w, _ in _subsystem {w}]

#DriverRow: {
	subsystem!: or(_subsystemWord)
	default:    *false | bool
	form?:      [...or(_formWord)] & list.UniqueItems()
	access?:    [...or(_accessWord)] & list.UniqueItems()
	capture?:   [...or(_captureWord)] & list.UniqueItems()
	grant?:     {[or(_leasableWord)]: or(_leaseModeWord)}
	tool?:      #Text
}
// ONE declaration, carrying the row pattern and the rows (see `_device` above): `_driverWord`
// derives DESIGN's fourteen drivers, and every row is a `#DriverRow`.
_driver: {
	[#Word]: #DriverRow
	podman:         {subsystem: "pod", default: true, tool: "podman", form: ["image"], access: ["exec", "copy", "log", "console", "forward"], capture: ["filesystem", "process"], grant: {gpu: "shared", device: "shared"}}
	docker:         {subsystem: "pod", tool: "docker", form: ["image"], access: ["exec", "copy", "log", "console", "forward"], capture: ["filesystem"], grant: {gpu: "shared", device: "shared"}}
	containerd:     {subsystem: "pod", form: ["image"], access: ["exec", "copy", "log", "console", "forward"], capture: ["filesystem"], grant: {gpu: "shared", device: "shared"}}
	libvirt:        {subsystem: "vm", default: true, tool: "virsh", form: ["disk"], access: ["exec", "copy", "log", "console", "display", "forward", "power"], capture: ["filesystem", "process"], grant: {gpu: "exclusive", device: "exclusive"}}
	qemu:           {subsystem: "vm", form: ["disk"], access: ["exec", "copy", "log", "console", "display", "forward", "power"], capture: ["filesystem"], grant: {gpu: "exclusive", device: "exclusive"}}
	krun:           {subsystem: "vm", form: ["image"], access: ["copy", "log", "console", "forward"], capture: ["filesystem"], grant: {gpu: "shared"}}
	firecracker:    {subsystem: "vm", form: ["disk"], access: ["exec", "copy", "log", "console", "forward", "power"], capture: ["filesystem", "process"]}
	kind:           {subsystem: "kubernetes", default: true, tool: "kind"}
	k3s:            {subsystem: "kubernetes", tool: "k3s"}
	pi:             {subsystem: "agent", default: true, tool: "pi", form: ["image"], access: ["exec", "console"]}
	tmux:           {subsystem: "agent", tool: "tmux", form: ["image"], access: ["exec", "console"]}
	secret_service: {subsystem: "secret", default: true}
	gpg:            {subsystem: "secret", tool: "gpg"}
	file:           {subsystem: "secret"}
} @vocab(spec)
_driverWord: [for w, _ in _driver {w}]

// ── §4.2: the roles a registry key names (D-ROLE-6), the last row of the table below ──
_role: {kind: {}, verb: {}, subcommand: {}, type: {}, api: {}} @vocab(spec)
_roleWord: [for w, _ in _role {w}]

// ── §8: one table of capability types; a need and a provide are rows of it ──
//
// A capability type: what its entries are keyed by, what a need entry and a provide entry add,
// whether it is a leasable host resource, and — for a type whose keys name nodes — the kinds they
// may name (`ref`, S9). Only a leasable type's entry is `exclusive` or not (D-HW-8), so `exclusive`
// on any other type is not a field.
#CapabilityRow: {
	key!:     _
	need?:    _
	provide?: _
	use?:     _ // what a `use:` entry carries: its need entry's fields, and a `default` where the type has a value
	lease:    *false | bool
	ref?:     [...string]
}
// ONE declaration: the pattern and the rows live in one literal (see `_device`), and the five
// role rows are DERIVED from `_role` — a role added there appears here, so the two cannot drift.
_capability: {
	[#Word]: #CapabilityRow
	env:        {key: #EnvName, need: {}, use: #EnvUse, provide: #EnvProvide}
	parameter:  {key: #Name, need: #ParameterNeed, use: #ParameterUse, provide: #ParameterProvide} // D-SCH-7
	secret:     {key: #Name, need: {}, use: #SecretUse}
	endpoint:   {key: #Name, need: #EndpointNeed, use: #EndpointNeed, provide: #Endpoint}
	network:    {key: #Name, need: {}, use: {}, provide: {}}
	harness:    {key: #Ref, need: {}, use: {}, ref: ["harness"]}
	gpu:        {key: or(_gpuWord), need: {}, use: {}, lease: true}
	device:     {key: or(_deviceWord), need: {}, use: {}, lease: true}
	cpu:        {key: or(_cpuWord), need: #CountNeed, use: #CountNeed, lease: true}
	memory:     {key: or(_memoryWord), need: #SizeNeed, use: #SizeNeed, lease: true}
	storage:    {key: or(_storageWord), need: #SizeNeed, use: #SizeNeed, lease: true}
	mount:      {key: #Name, need: #MountNeed, use: #MountNeed, lease: true}
	nesting:    {key: or(_nestingWord), need: {}, use: {}}
	isolation:  {key: or(_isolationWord), need: {}, use: {}} // D-HW-7: a boundary is a need, never a runtime setting
	privilege:  {key: or(_privilegeWord), need: {}, use: {}}
	driver_api: {key: or(_driverWord), need: {}, use: {}}
	for r, _ in _role {(r): {key: #Word, need: {}, use: {}, provide: {}}}
} @vocab(spec)
_capabilityWord: [for w, _ in _capability {w}]
_leasableWord: [for w, r in _capability if r.lease {w}]

// D-CAP-1: a capability's strength is the key it is written under — `need:` MUST be set, `use:` CAN
// be set — never a field of the entry. Only a `use:` entry carries a `default`.
#LeaseFields: {exclusive: *false | bool} // D-HW-8
#Need: {for t, c in _capability if c.need != _|_ {
	(t)?: {[c.key]: {c.need, if c.lease {#LeaseFields}}}
}}
#Use: {for t, c in _capability if c.use != _|_ {
	(t)?: {[c.key]: {c.use, if c.lease {#LeaseFields}}}
}}
#Provide: {for t, c in _capability if c.provide != _|_ {(t)?: {[c.key]: c.provide}}}

#EnvProvide: {value!: #InitNeutral} // D-CANDY-3
#EnvUse:     {default?: #InitNeutral}

// A parameter: a value a node takes from whoever references it. A candy's is a bounded choice,
// never a substitution (D-SCH-7); a pipeline's is any text.
#ParameterNeed:    {description?: #Text, one_of?: #TextSet}
#ParameterUse:     {#ParameterNeed, default?: #Text}
#ParameterProvide: {value!: #Text}
// D-SCH-7: a candy's parameter is a bounded choice, so its entry carries `one_of`.
#CandyParameterNeed: {#ParameterNeed, one_of!: #TextSet}
#CandyParameterUse:  {#ParameterUse, one_of!: #TextSet}
#CandyNeed: {#Need, parameter?: {[#Name]: #CandyParameterNeed}} // D-SCH-7
#CandyUse:  {#Use, parameter?: {[#Name]: #CandyParameterUse}}   // D-SCH-7

#EndpointNeed: {protocol!: or(_protocolWord)}
// The three LEASABLE shapes declare `exclusive` themselves, and that is the port of DESIGN's
// `{c.need..., #LeaseFields...}` rather than a weakening of it. DESIGN opens the closed
// definition AT THE EMBEDDING SITE with the postfix `...`, which the pinned CUE v0.16.1 refuses
// without @experiment(explicitopen); merging `#LeaseFields` into a CLOSED definition without it
// evaluates to `exclusive: field not allowed` -- a BOTTOM need entry, which the generator can
// only spell `map[string]any` and which would make every `need: {cpu: ...}` document
// unvalidatable. Declaring the lease field on the definition is the CUE-v0.16.1 spelling of the
// same net shape: `#Need`/`#Use` still add it only for a `lease: true` row, and these four rows
// (cpu, memory, storage, mount) are exactly the leasable ones.
#CountNeed:    {count!: #Count, exclusive: *false | bool}
#SizeNeed:     {size!: #Size, exclusive: *false | bool}
#MountNeed: {
	host!:      #AbsPath
	path!:      #AbsPath
	writable:   *false | bool
	exclusive:  *false | bool
}

// An endpoint the content serves: its port and transport, and the protocol, path and host name it
// is reached by. A deployment publishes endpoints by name (D-VAL-13).
#Endpoint: {
	port!:     #Port
	transport: *"tcp" | or(_transportWord)
	protocol?: or(_protocolWord)
	path?:     #AbsPath
	host?:     #Hostname
}
