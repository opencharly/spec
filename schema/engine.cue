// CUE schema for the `engine` provider class — the container-engine contract
// served by the compiled-in engine:podman / engine:docker providers and the
// out-of-tree engine:nerdctl plugin.
//
// WHY A CLASS: before this, the container engine was a bare string
// ("podman"|"docker"|"auto") threaded through ~170 literal sites and ~30
// switch statements across spec/sdk/core. Adding another engine meant editing
// every one of them. The engine class makes the engine a PROVIDER: core asks a
// provider for capability data and argv, instead of switching on a name.
//
// The split (kernel/plugin boundary law):
//   - engine NAME -> BINARY is data, so #EngineName / #EngineBinary below live
//     in spec and every authored engine field references them (one vocabulary).
//   - engine BEHAVIOR is provider-served: an engine provider answers the ops in
//     this file. podman/docker are compiled-in (needed before project plugins
//     load); nerdctl is out-of-process.
//
// Every field here is part of the published provider contract: the provider's
// served schema is spliced over the RPC by Describe, so a provider must not
// invent fields outside these defs.

// #EngineName — the CLOSED engine vocabulary. THE single source: every authored
// engine field (`candy.engine`, `deploy.engine`, the seam request/reply
// envelopes) references this def, so adding an engine is one edit here plus
// `task cue:gen`, never a sweep of literal unions.
//
// "auto" is a RESOLUTION selector (pick the best installed engine), never a
// provider word — no engine:auto provider exists. It is resolved by
// ResolveRuntime before any engine provider is consulted, so it is a runtime
// value, not an authored union member here.
#EngineName: ("podman" | "docker" | "nerdctl")

// #EngineRunMode — how a deployment persists and is supervised on the host.
//   quadlet      — podman's systemd generator: a .container/.pod unit under
//                  ~/.config/containers/systemd/ (podman only).
//   systemd-unit — a generated .service wrapping the engine CLI (nerdctl; no
//                  quadlet equivalent exists).
//   direct       — an ephemeral argv launch with no unit (docker today).
#EngineRunMode: ("quadlet" | "systemd-unit" | "direct")

// #EngineCapability — the static facts about an engine, answered by OpDescribe.
// These drive every remaining "does this engine support X" branch in core, so
// no caller switches on the engine name again.
#EngineCapability: {
	// The engine's own name (mirrors the provider word).
	name!: #EngineName @go(Name)
	// The CLI binary the engine is driven through (podman / docker / nerdctl).
	binary!: string & !="" @go(Binary)
	// A shell probe that exits 0 when the engine is installed on a host.
	// Evaluated by the host, never by the provider (it is a host fact).
	detect_probe?: string @go(DetectProbe)
	// supports_pods — the engine has a first-class pod primitive (podman .pod).
	// false means pod-style sharing is emulated with a shared network namespace.
	supports_pods!: bool @go(SupportsPods)
	// supports_secrets — the engine has a native secret store (podman secret).
	// false means credentials are delivered as env/file (docker/nerdctl).
	supports_secrets!: bool @go(SupportsSecrets)
	// supports_userns_keepid — per-container uid mapping exists (podman
	// --userns=keep-id). false means the workload must be launched as
	// container-uid-0 under a rootless single-userns engine to share host files
	// with the invoking user (nerdctl rootless).
	supports_userns_keepid!: bool @go(SupportsUsernsKeepID)
	// supports_rootless — the engine runs without host root.
	supports_rootless!: bool @go(SupportsRootless)
	// run_mode — the persistence/supervision mode this engine uses on a host.
	run_mode!: #EngineRunMode @go(RunMode)
	// gpu_arg_style — the vendor-GPU passthrough flag family.
	//   cdi  — podman's `--device nvidia.com/gpu=all`
	//   gpus — docker/nerdctl's `--gpus all`
	gpu_arg_style!: ("cdi" | "gpus") @go(GPUArgStyle)
	// userns_keepid_arg — the per-container keep-id argv when supported (podman
	// `--userns=keep-id`); empty otherwise.
	userns_keepid_arg?: string @go(UsernsKeepIDArg)
	// workload_user — the in-container user a workload must run as for
	// host-identical file sharing. For podman keep-id this is the invoking user;
	// for rootless single-userns engines (nerdctl) it is "0" because container-uid
	// 0 IS the invoking host user in the rootless userns.
	workload_user?: string @go(WorkloadUser)
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
// persistent supervision unit for a deployment (quadlet .container/.pod for
// podman, a systemd .service wrapping the CLI for nerdctl). Names/paths are
// host-resolved; the provider returns file contents keyed by absolute path.
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
	// engine_specific carries whatever the engine's own generator needs; quadlet
	// ignores it, the systemd-unit emitter uses it for the wrapped CLI argv.
	engine_specific?: #StrMap @go(EngineSpecific)
}
#EngineUnitReply: {
	// files maps an absolute path to its rendered contents.
	files?: {[string]: string} @go(Files)
	error?: string             @go(Error)
}

// #EngineNetworkEnsureRequest / #EngineNetworkEnsureReply — the `network_ensure`
// op: make the engine's shared container network exist (podman netavark+aardvark
// `charly`; nerdctl CNI network).
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
