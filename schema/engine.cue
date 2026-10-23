// CUE schema for the `engine` provider class — the container-engine contract
// served by the compiled-in engine:podman / engine:docker providers and the
// out-of-tree engine:containerd plugin.
//
// WHY A CLASS: before this, the container engine was a bare string
// ("podman"|"docker"|"auto") threaded through ~170 literal sites and ~30
// switch statements across spec/sdk/core. Adding another engine meant editing
// every one of them. The engine class makes the engine a PROVIDER: core asks a
// provider for capability data and argv, instead of switching on a name.
//
// The split (kernel/plugin boundary law):
//   - the engine VOCABULARY (which engine words, which run modes, which modes are
//     unit-supervised) is data, owned here by #EngineName / #EngineRunMode /
//     #EngineUnitRunModes. `charly task cue-gen` emits them as spec.EngineNames /
//     spec.EngineRunModes / spec.EngineUnitRunModeWords, and each consumer
//     predicate reads its OWN list: IsEngineName reads spec.EngineNames,
//     IsRunMode reads spec.EngineRunModes, and IsUnitRunMode reads
//     spec.EngineUnitRunModeWords. The authored candy.engine/deploy.engine
//     fields are typed #EngineName.
//     The op-envelope `engine` fields below stay plain `string` on purpose:
//     they carry the RESOLVED or still-`auto`/empty engine at the wire boundary,
//     which is a superset of the authored vocabulary — a `#EngineName` there
//     would reject the legitimate pre-resolution value.
//   - the engine CAPABILITY FACTS (pods? secrets? keep-id? run mode? gpu style?)
//     live in one Go table keyed by those words (container.engineCapabilities),
//     typed by the generated spec.EngineCapability below. EngineBinary and
//     EngineCapabilityFor read that table; a drift test asserts each row's
//     Name/RunMode are members of the emitted vocabularies.
//     So adding an engine is one edit to #EngineName plus one table row (the
//     table row is what supplies the CLI binary and the native-feature facts);
//     adding a RUN MODE is one edit to #EngineRunMode plus, if it is
//     unit-supervised, one entry in #EngineUnitRunModes.
//   - engine BEHAVIOR is provider-served: an engine provider answers the ops in
//     this file. podman/docker are compiled-in (needed before project plugins
//     load); containerd is out-of-process.
//
// Every field here is part of the published provider contract: the provider's
// served schema is spliced over the RPC by Describe, so a provider must not
// invent fields outside these defs.

// #EngineName — the CLOSED engine vocabulary. THE single source for the engine
// words: the authored candy.engine/deploy.engine fields are this def, and
// `charly task cue-gen` emits it as spec.EngineNames, from which IsEngineName derives.
// EngineBinary/EngineCapabilityFor answer from the capability table keyed by
// these same words (drift-tested against spec.EngineNames).
//
// "auto" is a RESOLUTION selector (pick the best installed engine), never a
// provider word — no engine:auto provider exists. It is resolved by
// ResolveRuntime before any engine provider is consulted, so it is a runtime
// value, not an authored union member here.
//
// The word `containerd` means "containerd, driven through the nerdctl CLI",
// rootful OR rootless. `nerdctl` is the CLI binary, never an engine word: the
// former `engine: nerdctl` spelling is deleted (R5, no alias, no dual path).
#EngineName: ("podman" | "docker" | "containerd")

// #EngineBinaries — the engine WORD → CLI BINARY mapping. The two are NOT the
// same word: containerd is driven through nerdctl, while podman/docker use their
// own binary. The mapping lives in the contract module so the capability table
// (container/engine.go) and the exec layer (exec/deploy_executor_nested.go) read
// ONE source without an import cycle. @go(-) suppresses a useless generated
// struct; `charly task cue-gen` emits spec.EngineBinaries.
#EngineBinaries: {
	podman:     "podman"
	docker:     "docker"
	containerd: "nerdctl"
} @go(-)

// #EngineRunMode — how a deployment persists and is supervised on the host.
//   quadlet      — podman's systemd generator: a .container/.pod unit under
//                  ~/.config/containers/systemd/ (podman only).
//   systemd-unit — a generated .service wrapping the engine CLI (containerd's
//                  nerdctl CLI; no quadlet equivalent exists).
//   direct       — an ephemeral argv launch with no unit (docker today).
// `charly task cue-gen` emits this as spec.EngineRunModes; ValidateRunMode derives
// from it, and container.IsRunMode reads it.
#EngineRunMode: ("quadlet" | "systemd-unit" | "direct")

// #EngineUnitRunModes — the SUBSET of #EngineRunMode that is supervised by a
// generated unit file (as opposed to an ephemeral argv launch). This is the
// data fact that `direct` is not unit-supervised, so a caller never hand-lists
// {"quadlet","systemd-unit"} — that pair was a drift-prone second source.
// @go(-) suppresses a useless generated Go type; `charly task cue-gen` emits the list
// as spec.EngineUnitRunModeWords, which container.IsUnitRunMode reads.
#EngineUnitRunModes: ["quadlet", "systemd-unit"] @go(-)

// #RootfulMode — the CLOSED rootful-launch vocabulary shared by the
// `engine.rootful` settings key, its `CHARLY_ENGINE_ROOTFUL` env mirror, and the
// per-deploy `rootful:` selector. `auto` asks the resolver to pick from the
// engine's capability facts (`supports_rootful` / `privilege_prefix`); the rest
// are explicit operator choices. `charly task cue-gen` emits it as
// spec.RootfulModes, so a resolver validates a flag/env/settings value without a
// hand-listed word set.
#RootfulMode: ("auto" | "machine" | "sudo" | "native")

// #EngineCapability — the static facts about an engine, answered by OpDescribe.
// This is the PUBLISHED provider contract: the fields a provider fills and the
// consumers read.
//
// AUTHORITY (the one home): `container.engineCapabilities` is the authoritative
// table for the COMPILED-IN podman/docker engines and the built-in consumers
// (EngineBinary/GPURunArgs/EngineRunModeFor/ImageExistsArgv/WorkloadUserFor). This
// def is the shape an out-of-tree engine provider (plugin-containerd) answers over
// Describe; the runtime compares/consumes that answer through the same struct,
// so the table and the wire answer are one type, not two divergent copies. The
// op envelopes below (#EngineBinaryRequest …) are for a provider that chooses to
// answer an op instead of Describe.
//
// In this change the capability table + EngineBinary/GPURunArgs/EngineRunModeFor/
// ImageExistsArgv consumers land; the class name (#ProviderClassNames) and the op
// envelopes below are the STABLE wire contract the compiled-in podman/docker
// providers and the out-of-tree plugin-containerd bind to. `#EngineCapability` is
// the one type shared by the table and the wire answer. The pod/secret/keep-id
// fields (SupportsPods / SupportsSecrets / SupportsUsernsKeepID /
// UsernsKeepIDArg / WorkloadUserRootless / WorkloadUserRootful) are declared so
// that contract is complete; the compiled-in EngineBinary consumer reads
// `binary`, and the remaining fields' readers are the engine providers
// themselves.
//
// The containerd row is the reason several of these fields exist as DATA: one
// engine word covers a rootful daemon, a rootless daemon, a socket address, a
// namespace, a privilege prefix and a set of runtime handlers (crun/krun/runc/
// nvidia). A run plan reads them instead of spelling `--runtime <literal>` or
// assuming rootless.
#EngineCapability: {
	// The engine's own name (mirrors the provider word).
	name!: #EngineName @go(Name)
	// The CLI binary the engine is driven through (podman / docker / nerdctl).
	// containerd is driven through nerdctl; the ENGINE word remains containerd.
	binary!: string & !="" @go(Binary)
	// A shell probe that exits 0 when the engine is installed on a host.
	// Evaluated by the host, never by the provider (it is a host fact).
	detect_probe?: string @go(DetectProbe)
	// supports_pods — the engine has a first-class pod primitive (podman .pod).
	// false means pod-style sharing is emulated with a shared network namespace.
	supports_pods!: bool @go(SupportsPods)
	// supports_secrets — the engine has a native secret store (podman secret).
	// false means credentials are delivered as env/file (docker/containerd).
	supports_secrets!: bool @go(SupportsSecrets)
	// supports_userns_keepid — per-container uid mapping exists (podman
	// --userns=keep-id). false means the workload must be launched as
	// container-uid-0 under a rootless single-userns engine to share host files
	// with the invoking user (containerd rootless).
	supports_userns_keepid!: bool @go(SupportsUsernsKeepID)
	// supports_rootless — the engine runs without host root.
	supports_rootless!: bool @go(SupportsRootless)
	// supports_rootful — the engine can also be driven against a host-root daemon
	// (directly or through privilege_prefix). containerd supports BOTH; a
	// capability consumer must not assume the rootless mode.
	supports_rootful!: bool @go(SupportsRootful)
	// run_mode — the persistence/supervision mode this engine uses on a host.
	run_mode!: #EngineRunMode @go(RunMode)
	// gpu_arg_style — the vendor-GPU passthrough flag family.
	//   cdi  — podman's `--device nvidia.com/gpu=all`
	//   gpus — docker/containerd's `--gpus all`
	gpu_arg_style!: ("cdi" | "gpus") @go(GPUArgStyle)
	// userns_keepid_arg — the per-container keep-id argv when supported (podman
	// `--userns=keep-id`); empty otherwise.
	userns_keepid_arg?: string @go(UsernsKeepIDArg)
	// workload_user_rootless — the in-container user a workload must run as in the
	// ROOTLESS mode for host-identical file sharing. For rootless single-userns
	// engines (containerd via nerdctl) it is "0" because container-uid 0 IS the
	// invoking host user in the rootless userns. Empty means "do not force a user".
	workload_user_rootless?: string @go(WorkloadUserRootless)
	// workload_user_rootful — the in-container user for the ROOTFUL mode. Empty
	// means "do not force a user": the workload runs as the image's declared user
	// instead of inheriting the rootless uid-0 mapping. Splitting the field is what
	// stops the capability from assuming every engine is rootless.
	workload_user_rootful?: string @go(WorkloadUserRootful)
	// address — the engine daemon endpoint (containerd's socket). Empty for an
	// engine with no separate daemon endpoint (podman/docker's default context).
	address?: string @go(Address)
	// namespace — the engine's default namespace. containerd's uniform default is
	// "default"; a per-deploy/config override is resolved in code (deploy > config
	// > this default), never re-spelled here.
	namespace?: string @go(Namespace)
	// privilege_prefix — the argv prefix a ROOTFUL target uses to reach the daemon
	// (e.g. `sudo -n`). Empty when the invoking user already has access. The
	// prefix is data so a consumer never hard-codes an escalation command.
	privilege_prefix?: string @go(PrivilegePrefix)
	// substrates — the deploy substrate kinds this engine may host, as DATA. The
	// loader reads this instead of a word literal: containerd hosts ["pod"]; a
	// kind:vm through the krun backend is reached by the VM-backend class, not by
	// widening this list. An engine may host only these kinds and nothing else.
	substrates!: [...("pod" | "vm")] @go(Substrates)
	// runtime_handlers — the OCI runtime handlers this engine knows (containerd:
	// crun/krun/runc/nvidia). A run plan selects `--runtime` from this set; the
	// handler word is DATA, never a literal in the plan builder.
	runtime_handlers!: [...string] @go(RuntimeHandlers)
	// image_exists_argv — the subcommand pair that answers "is this image in the
	// local store?" (podman `image exists`, docker/containerd `image inspect`, since
	// docker has no `image exists`). A capability fact so local-image probes do
	// not switch on the engine name.
	image_exists_argv: [...string] @go(ImageExistsArgv)
	// no_remove_with_detach — the engine REJECTS `-d` (detach) together with `--rm`
	// (auto-remove). containerd's nerdctl CLI does ("flags -d and --rm cannot be
	// specified together", measured live on nerdctl 2.3.5); podman/docker accept
	// both. A capability fact so a detached-argv builder drops `--rm` where it is
	// illegal (the container is then cleaned by the unit's ExecStop / an explicit
	// rm) instead of switching on the engine name.
	no_remove_with_detach: bool @go(NoRemoveWithDetach)
	// systemd_mode_arg — the argv token that turns the engine's systemd mode ON
	// for a container whose init declares `systemd_mode: true` (podman
	// `--systemd=always`, nerdctl `--systemd=true`). EMPTY when the engine has
	// no such mode (docker): the deploy then omits it rather than inventing a
	// flag, and only the stop signal is emitted.
	systemd_mode_arg?: string @go(SystemdModeArg)
}

// #EngineBinaryRequest / #EngineBinaryReply — the `binary` op: resolve the CLI
// binary for an engine name. Split out because the name->binary mapping is pure
// data that callers need before any provider is connected.
#EngineBinaryRequest: {
	engine?: string @go(Engine)
}
#EngineBinaryReply: {
	binary?: string @go(Binary)
}

// #EngineGPURunArgsRequest / #EngineGPURunArgsReply — the `gpu_args` op: the
// engine's vendor-GPU passthrough argv.
#EngineGPURunArgsRequest: {
	engine?: string @go(Engine)
}
#EngineGPURunArgsReply: {
	args?: [...string] @go(Args)
}

// #EngineStartPlanRequest / #EngineStartPlanReply — the `start_plan` op: the
// engine-specific container launch argv and mode for a deploy. The host passes
// the resolved knobs; the provider builds the argv so no caller assembles
// engine flags.
#EngineStartPlanRequest: {
	name?:         string @go(Name)
	image?:        string @go(Image)
	engine?:       string @go(Engine)
	run_mode?:     string @go(RunMode)
	network?:      string @go(Network)
	workload_user?: string @go(WorkloadUser)
	// rootful selects which workload-user fact applies (rootless vs rootful) and
	// whether the engine target needs a privilege prefix. It is an explicit knob so
	// the plan builder never assumes the rootless mode.
	rootful?: bool @go(Rootful)
	// runtime is the OCI runtime handler word (crun/krun/runc/nvidia), validated by
	// the caller against the engine capability's runtime_handlers. When set, the
	// plan passes it as `--runtime`; never a hard-coded handler literal.
	runtime?:      string @go(Runtime)
	keep_id?:      bool   @go(KeepID)
	userns_host?:  bool   @go(UsernsHost)
	detach?:       bool   @go(Detach)
	ports?:        [...string] @go(Ports)
	volumes?:      [...string] @go(Volumes)
	env?:          #StrMap @go(Env)
	env_files?:    [...string] @go(EnvFiles)
	extra_args?:   [...string] @go(ExtraArgs)
	image_args?:   [...string] @go(ImageArgs)
}
#EngineStartPlanReply: {
	argv?:     [...string] @go(Argv)
	run_mode?: string      @go(RunMode)
	error?:    string      @go(Error)
}

// #EngineUnitRequest / #EngineUnitReply — the `unit_emit` op: render the
// non-quadlet persistent supervision unit for a deployment — a systemd .service
// wrapping the engine CLI (containerd's mode). Names/paths are host-resolved; the
// provider returns file contents keyed by absolute path.
//
// QUADLET IS NOT RENDERED HERE: podman's run mode is `quadlet`, and its
// .container/.pod units are emitted by podman's OWN generator (the caller's
// generator), not by this op. A request whose run mode is quadlet, or is not a
// unit mode at all, returns an EMPTY reply (no files) — the caller's quadlet
// generator owns that form. This op answers only the engine that has no native
// generator.
#EngineUnitRequest: {
	name?:       string @go(Name)
	run_mode?:   string @go(RunMode)
	start_argv?: [...string] @go(StartArgv)
	stop_argv?:  [...string] @go(StopArgv)
	exec_start_pre?:  [...string] @go(ExecStartPre)
	description?:     string          @go(Description)
	after?:           [...string] @go(After)
	wants?:           [...string] @go(Wants)
	restart?:         string          @go(Restart)
	working_dir?:     string          @go(WorkingDir)
	env?:             #StrMap         @go(Env)
	// engine_specific carries whatever engine-specific extras a future unit
	// generator needs (a quadlet-only knob, an engine-specific ordering hint, …).
	// It is a declared CARRIER: the systemd-unit servant builds ExecStart from
	// start_argv and does not read it today, and podman's quadlet is emitted by
	// podman's OWN generator, not this op.
	engine_specific?: #StrMap @go(EngineSpecific)
}
#EngineUnitReply: {
	// files maps an absolute path to its rendered contents.
	files?: {[string]: string} @go(Files)
	error?: string             @go(Error)
}

// #EngineNetworkEnsureRequest / #EngineNetworkEnsureReply — the `network_ensure`
// op: make the engine's shared container network exist (podman netavark+aardvark
// `charly`; containerd's nerdctl CNI network).
#EngineNetworkEnsureRequest: {
	name?:       string @go(Name)
	engine?:     string @go(Engine)
	dns?:        [...string] @go(DNS)
	dns_search?: [...string] @go(DNSSearch)
}
#EngineNetworkEnsureReply: {
	network?: string   @go(Network)
	argv?:    [...string] @go(Argv)
	error?:   string   @go(Error)
}

// #EngineDescribeReply — the capability envelope served for the engine class.
// The provider's Describe returns this; core resolves it once and consults it
// for every capability question.
#EngineDescribeReply: {
	capability?: #EngineCapability @go(Capability,optional=nillable)
	error?:      string            @go(Error)
}
