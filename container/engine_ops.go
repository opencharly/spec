package container

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/opencharly/spec/ops"
	"github.com/opencharly/spec/spec"
)

// engine_ops.go — the PURE servant for the `engine` provider class declared in
// schema/engine.cue. Every op the class declares is answered HERE, from the
// capability table + the existing host-side argv/unit logic, so the declared
// class has an in-tree servant (not a shipped-but-unconsumed contract).
//
// The kernel/plugin boundary law places engine BEHAVIOR on a provider: this is
// the placement-agnostic op BODY. A compiled-in charly provider
// (engine:podman / engine:docker) and an out-of-process engine:nerdctl plugin
// both wrap InvokeEngineOp, so an op is written once and served from either
// placement — the same F8 property every other class relies on. The spec module
// carries no project state, so the servant is pure: it takes the op envelope's
// params as JSON and returns the reply as JSON, exactly what a Provider.Invoke
// hands across the wire.
//
// The engine data facts (binary, run mode, gpu style, probes) come from
// engineCapabilities via EngineCapabilityFor — the ONE table. The GPU flag
// family is defined ONCE in engineGPUArgsRaw below; the public GPURunArgs is its
// typed accessor (GPURunArgs → InvokeEngineOp → engineGPUArgsRaw), so there is no
// second copy of the flag form.

// EngineOps returns the op selectors the engine class serves. A provider
// advertises exactly these; a test asserts each is answered by InvokeEngineOp,
// so a newly-declared op cannot ship unserved.
func EngineOps() []string {
	return []string{
		ops.OpEngineDescribe,
		ops.OpEngineBinary,
		ops.OpEngineGPURunArgs,
		ops.OpEngineStartPlan,
		ops.OpEngineUnitEmit,
		ops.OpEngineNetworkEnsure,
	}
}

// InvokeEngineOp dispatches one engine-class op for the provider word `reserved`
// (podman/docker/nerdctl) and returns the reply as JSON. It is the placement-
// agnostic body both a compiled-in charly provider and an out-of-process plugin
// wrap. An unknown op is a hard error (never a silent empty reply), so a
// dispatch gap is visible.
//
// `params` is the raw spec.Engine*Request JSON the caller marshalled; the
// resolved engine word rides INSIDE the request (the envelopes carry an
// `engine`/no field), so `reserved` is only the fallback when the request omits
// it — a caller may invoke any engine word through any engine provider (the
// provider is a servant of the CLASS, keyed by the request's engine).
func InvokeEngineOp(reserved, op string, params json.RawMessage) (json.RawMessage, error) {
	switch op {
	case ops.OpEngineDescribe:
		return engineDescribe(reserved)
	case ops.OpEngineBinary:
		var req spec.EngineBinaryRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("engine binary op: %w", err)
			}
		}
		return marshalReply(spec.EngineBinaryReply{Binary: engineBinaryRaw(engineOr(req.Engine, reserved))})
	case ops.OpEngineGPURunArgs:
		var req spec.EngineGPURunArgsRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("engine gpu_args op: %w", err)
			}
		}
		return marshalReply(spec.EngineGPURunArgsReply{Args: engineGPUArgsRaw(engineOr(req.Engine, reserved))})
	case ops.OpEngineStartPlan:
		var req spec.EngineStartPlanRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("engine start_plan op: %w", err)
			}
		}
		return engineStartPlan(engineOr(req.Engine, reserved), req)
	case ops.OpEngineUnitEmit:
		var req spec.EngineUnitRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("engine unit_emit op: %w", err)
			}
		}
		// The engine word is the provider's (the envelope carries no engine field —
		// its run_mode is a MODE, not an engine). Using run_mode here would resolve a
		// mode word as an engine.
		return engineUnitEmit(reserved, req)
	case ops.OpEngineNetworkEnsure:
		var req spec.EngineNetworkEnsureRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("engine network_ensure op: %w", err)
			}
		}
		return engineNetworkEnsure(engineOr(req.Engine, reserved), req)
	default:
		return nil, fmt.Errorf("engine op %q is not served (served: %v)", op, EngineOps())
	}
}

// engineOr returns the request's engine word when present, else the provider's
// reserved word — so a caller may leave the envelope's engine empty and get the
// provider's own engine.
func engineOr(reqEngine, reserved string) string {
	if reqEngine != "" {
		return reqEngine
	}
	return reserved
}

func marshalReply(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// engineBinaryRaw is the name→binary LEAF: it reads the capability table directly
// (no dispatch), so the `binary` op body can call it without recursing into
// EngineBinary (which is the op's typed accessor). An unknown/empty word resolves
// to the ONE default engine, matching EngineBinary.
func engineBinaryRaw(engine string) string {
	if c, ok := EngineCapabilityFor(engine); ok {
		return c.Binary
	}
	return spec.DefaultContainerEngine
}

// engineGPUArgsRaw is the gpu-args LEAF (capability-table read, no dispatch), used
// by the `gpu_args` op body so it never recurses into GPURunArgs.
func engineGPUArgsRaw(engine string) []string {
	if c, ok := EngineCapabilityFor(engine); ok && c.GPUArgStyle == "cdi" {
		return []string{"--device", "nvidia.com/gpu=all"}
	}
	return []string{"--gpus", "all"}
}

// engineDescribe answers OpEngineDescribe: the capability envelope. An unknown
// non-empty word resolves to the default engine's capability (the same single-home
// resolution every consumer uses), so it is NOT an error; the Error reply is
// produced only when EngineCapabilityFor finds no engine at all — "auto" with no
// engine installed — matching the envelope's own contract rather than a Go error.
func engineDescribe(engine string) (json.RawMessage, error) {
	c, ok := EngineCapabilityFor(engine)
	if !ok {
		return marshalReply(spec.EngineDescribeReply{Error: fmt.Sprintf("unknown engine %q", engine)})
	}
	return marshalReply(spec.EngineDescribeReply{Capability: &c})
}

// engineStartPlan answers OpEngineStartPlan: the engine-specific container launch
// argv. The plan's spike-proven sharing rule is applied here from the capability
// table — an engine without keep-id (nerdctl rootless) launches the workload as
// its declared WorkloadUser (container-uid 0 == the invoking host user), while a
// keep-id engine uses its UsernsKeepIDArg when the caller asked for keep_id.
func engineStartPlan(engine string, req spec.EngineStartPlanRequest) (json.RawMessage, error) {
	c, ok := EngineCapabilityFor(engine)
	if !ok {
		return marshalReply(spec.EngineStartPlanReply{Error: fmt.Sprintf("unknown engine %q", engine)})
	}
	argv := []string{c.Binary, "run"}
	if req.Detach {
		argv = append(argv, "-d")
	}
	if req.Name != "" {
		argv = append(argv, "--name", req.Name)
	}
	if req.Network != "" {
		argv = append(argv, "--network", req.Network)
	}
	// uid-identical sharing: keep-id engines map the invoking user; engines
	// without keep-id run as the declared workload user (0 == host user rootless).
	switch {
	case req.KeepID && c.SupportsUsernsKeepID && c.UsernsKeepIDArg != "":
		argv = append(argv, c.UsernsKeepIDArg)
	case !c.SupportsUsernsKeepID && c.WorkloadUser != "":
		if req.WorkloadUser == "" {
			req.WorkloadUser = c.WorkloadUser
		}
		argv = append(argv, "--user", req.WorkloadUser)
	case req.WorkloadUser != "":
		argv = append(argv, "--user", req.WorkloadUser)
	}
	if req.UsernsHost {
		argv = append(argv, "--userns", "host")
	}
	for _, p := range req.Ports {
		argv = append(argv, "-p", p)
	}
	for _, v := range req.Volumes {
		argv = append(argv, "-v", v)
	}
	for _, f := range req.EnvFiles {
		argv = append(argv, "--env-file", f)
	}
	for k, v := range req.Env {
		argv = append(argv, "-e", k+"="+v)
	}
	argv = append(argv, req.ExtraArgs...)
	if req.Image != "" {
		argv = append(argv, req.Image)
	}
	argv = append(argv, req.ImageArgs...)
	mode := req.RunMode
	if mode == "" {
		mode = string(c.RunMode)
	}
	return marshalReply(spec.EngineStartPlanReply{Argv: argv, RunMode: mode})
}

// engineUnitEmit answers OpEngineUnitEmit: the supervision unit file(s) keyed by
// absolute path. The QUADLET run mode is podman's own generator (no file from
// charly here — the caller emits the quadlet via its generator); the SYSTEMD-UNIT
// run mode wraps the CLI in a plain .service. This servant renders the
// systemd-unit form; a quadlet request returns no files (the quadlet generator is
// the caller's, not this pure servant's).
func engineUnitEmit(engine string, req spec.EngineUnitRequest) (json.RawMessage, error) {
	if _, ok := EngineCapabilityFor(engine); !ok {
		return marshalReply(spec.EngineUnitReply{Error: fmt.Sprintf("unknown engine %q", engine)})
	}
	// The decision input is the REQUEST's run mode (the envelope's declared field);
	// when the caller leaves it empty, the engine's own capability mode is the
	// default. Either way, quadlet — or any non-unit mode — yields no files here:
	// podman's quadlet generator emits that form, not this op.
	mode := req.RunMode
	if mode == "" {
		c, _ := EngineCapabilityFor(engine)
		mode = string(c.RunMode)
	}
	if !IsUnitRunMode(mode) || mode == "quadlet" {
		return marshalReply(spec.EngineUnitReply{})
	}
	if len(req.StartArgv) == 0 {
		return marshalReply(spec.EngineUnitReply{Error: "unit_emit: start_argv is empty"})
	}
	name := req.Name
	if name == "" {
		return marshalReply(spec.EngineUnitReply{Error: "unit_emit: name is required"})
	}
	text := renderSystemdUnit(req)
	return marshalReply(spec.EngineUnitReply{Files: map[string]string{
		"charly-" + name + ".service": text,
	}})
}

// renderSystemdUnit renders the generated user-unit text. It is deliberately a
// thin wrapper: the container lifecycle is the engine CLI's, systemd supervises
// the CLI process (the spike-proven persistence path for an engine with no
// quadlet generator).
func renderSystemdUnit(req spec.EngineUnitRequest) string {
	var b []byte
	add := func(s string) { b = append(b, s...) }
	add("[Unit]\n")
	desc := req.Description
	if desc == "" {
		desc = "charly deployment " + req.Name
	}
	add("Description=" + desc + "\n")
	for _, a := range req.After {
		add("After=" + a + "\n")
	}
	add("Wants=network-online.target\n")
	for _, w := range req.Wants {
		add("Wants=" + w + "\n")
	}
	add("\n[Service]\n")
	restart := req.Restart
	if restart == "" {
		restart = "on-failure"
	}
	add("Restart=" + restart + "\n")
	// ExecStartPre: one directive per entry (each is a full command line).
	for _, pre := range req.ExecStartPre {
		if pre != "" {
			add("ExecStartPre=" + pre + "\n")
		}
	}
	add("ExecStart=" + strings.Join(req.StartArgv, " ") + "\n")
	if len(req.StopArgv) > 0 {
		add("ExecStop=" + strings.Join(req.StopArgv, " ") + "\n")
	}
	if req.WorkingDir != "" {
		add("WorkingDirectory=" + req.WorkingDir + "\n")
	}
	if len(req.Env) > 0 {
		// Deterministic order so a re-render is a no-op (drift gate).
		keys := make([]string, 0, len(req.Env))
		for k := range req.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add("Environment=" + k + "=" + req.Env[k] + "\n")
		}
	}
	add("\n[Install]\n")
	add("WantedBy=default.target\n")
	return string(b)
}

// engineNetworkEnsure answers OpEngineNetworkEnsure: the argv that makes the
// shared container network exist (podman `network create`; nerdctl CNI
// `network create`). The caller runs the argv; the servant returns it.
func engineNetworkEnsure(engine string, req spec.EngineNetworkEnsureRequest) (json.RawMessage, error) {
	c, ok := EngineCapabilityFor(engine)
	if !ok {
		return marshalReply(spec.EngineNetworkEnsureReply{Error: fmt.Sprintf("unknown engine %q", engine)})
	}
	if req.Name == "" {
		return marshalReply(spec.EngineNetworkEnsureReply{Error: "network_ensure: name is required"})
	}
	argv := []string{c.Binary, "network", "create"}
	for _, d := range req.DNS {
		argv = append(argv, "--dns", d)
	}
	for _, s := range req.DNSSearch {
		argv = append(argv, "--dns-search", s)
	}
	argv = append(argv, req.Name)
	return marshalReply(spec.EngineNetworkEnsureReply{Network: req.Name, Argv: argv})
}
