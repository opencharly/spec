package container

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/opencharly/spec/ops"
	"github.com/opencharly/spec/spec"
)

// TestEveryDeclaredEngineOpIsServed is the block-2 gate: the `engine` provider
// class declares a fixed op set (schema/engine.cue), and every declared op MUST be
// answered by InvokeEngineOp — so the class is SERVABLE, not a contract shipped
// without a servant. It fails if an op is added to EngineOps() but not handled,
// or handled but dropped from the advertised set.
func TestEveryDeclaredEngineOpIsServed(t *testing.T) {
	want := map[string]bool{
		ops.OpEngineDescribe:      true,
		ops.OpEngineBinary:        true,
		ops.OpEngineGPURunArgs:    true,
		ops.OpEngineStartPlan:     true,
		ops.OpEngineUnitEmit:      true,
		ops.OpEngineNetworkEnsure: true,
	}
	got := map[string]bool{}
	for _, op := range EngineOps() {
		got[op] = true
	}
	if len(got) != len(want) {
		t.Fatalf("EngineOps() has %d ops, want %d (%v)", len(got), len(want), EngineOps())
	}
	for op := range want {
		if !got[op] {
			t.Errorf("declared engine op %q is not in EngineOps()", op)
		}
	}
	// Every advertised op must actually dispatch (no unserved op, no error).
	for _, engine := range spec.EngineNames {
		for op := range want {
			out, err := InvokeEngineOp(engine, op, json.RawMessage(`{}`))
			if err != nil {
				t.Errorf("InvokeEngineOp(%s, %s) errored: %v", engine, op, err)
			}
			if len(out) == 0 {
				t.Errorf("InvokeEngineOp(%s, %s) returned an empty reply", engine, op)
			}
		}
	}
	// An unknown op is a hard error, never a silent empty reply.
	if _, err := InvokeEngineOp("podman", "no-such-op", nil); err == nil {
		t.Error("InvokeEngineOp with an unknown op must error, not return empty")
	}
}

// TestEngineDescribeOp pins the describe reply shape: the capability envelope
// round-trips, and an unknown engine yields the reply's error field (not a Go
// error) — the envelope's own contract.
func TestEngineDescribeOp(t *testing.T) {
	out, err := InvokeEngineOp("podman", ops.OpEngineDescribe, nil)
	if err != nil {
		t.Fatalf("describe(podman) errored: %v", err)
	}
	var reply spec.EngineDescribeReply
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Fatalf("describe reply did not parse: %v", err)
	}
	if reply.Capability == nil {
		t.Fatal("describe(podman) returned no capability")
	}
	if reply.Capability.Binary != "podman" || reply.Capability.RunMode != "quadlet" {
		t.Errorf("describe(podman) capability = %+v, want binary=podman run_mode=quadlet", *reply.Capability)
	}

	// An unknown non-empty word resolves to the default engine's capability, the
	// SAME single-home resolution the empty word gets — so describe agrees with
	// EngineBinary/GPURunArgs/probe (one input, one engine).
	out, err = InvokeEngineOp("bogus", ops.OpEngineDescribe, nil)
	if err != nil {
		t.Fatalf("describe(bogus) must not be a Go error: %v", err)
	}
	var bogusReply spec.EngineDescribeReply
	if err := json.Unmarshal(out, &bogusReply); err != nil {
		t.Fatalf("describe(bogus) reply did not parse: %v", err)
	}
	if bogusReply.Capability == nil || bogusReply.Capability.Binary != spec.DefaultContainerEngine {
		t.Errorf("describe(bogus) = %+v, want the default engine %q's capability", bogusReply, spec.DefaultContainerEngine)
	}
}

// TestEngineBinaryAndGPUOps pins the two data ops against the ONE capability
// table, including the empty-word single-home resolution (block 1).
func TestEngineBinaryAndGPUOps(t *testing.T) {
	out, err := InvokeEngineOp("docker", ops.OpEngineBinary, nil)
	if err != nil {
		t.Fatal(err)
	}
	var brep spec.EngineBinaryReply
	if err := json.Unmarshal(out, &brep); err != nil {
		t.Fatal(err)
	}
	if brep.Binary != "docker" {
		t.Errorf("binary(docker) = %q, want docker", brep.Binary)
	}

	// The request's own engine field wins over the provider word.
	out, err = InvokeEngineOp("docker", ops.OpEngineBinary, json.RawMessage(`{"engine":"podman"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &brep); err != nil {
		t.Fatal(err)
	}
	if brep.Binary != "podman" {
		t.Errorf("binary with request engine=podman = %q, want podman (request wins)", brep.Binary)
	}

	// Empty engine resolves to the ONE default engine's GPU form — not docker's.
	out, err = InvokeEngineOp("", ops.OpEngineGPURunArgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	var grep spec.EngineGPURunArgsReply
	if err := json.Unmarshal(out, &grep); err != nil {
		t.Fatal(err)
	}
	want := GPURunArgs(spec.DefaultContainerEngine)
	if len(grep.Args) != len(want) || grep.Args[0] != want[0] {
		t.Errorf("gpu_args(\"\") = %v, want the default engine's %v", grep.Args, want)
	}
}

// TestEngineStartPlanOp pins the spike-proven sharing rule: a keep-id engine
// (podman) uses its keep-id argv when asked; an engine WITHOUT keep-id (nerdctl)
// launches the workload as its declared WorkloadUser (uid 0 == the host user).
func TestEngineStartPlanOp(t *testing.T) {
	// podman keep_id → the keep-id argv.
	out, err := InvokeEngineOp("podman", ops.OpEngineStartPlan,
		json.RawMessage(`{"engine":"podman","image":"alpine","name":"svc","detach":true,"keep_id":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var rep spec.EngineStartPlanReply
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.RunMode != "quadlet" {
		t.Errorf("start_plan(podman) run_mode = %q, want quadlet", rep.RunMode)
	}
	if !containsArg(rep.Argv, "--userns=keep-id") {
		t.Errorf("start_plan(podman,keep_id) argv = %v, want the keep-id argv", rep.Argv)
	}

	// nerdctl has no keep-id → the workload runs as the declared user (0).
	out, err = InvokeEngineOp("nerdctl", ops.OpEngineStartPlan,
		json.RawMessage(`{"engine":"nerdctl","image":"alpine","name":"svc","keep_id":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.RunMode != "systemd-unit" {
		t.Errorf("start_plan(nerdctl) run_mode = %q, want systemd-unit", rep.RunMode)
	}
	if !containsArgPair(rep.Argv, "--user", "0") {
		t.Errorf("start_plan(nerdctl) argv = %v, want --user 0 (uid 0 == host user rootless)", rep.Argv)
	}
	if containsArg(rep.Argv, "--userns=keep-id") {
		t.Errorf("start_plan(nerdctl) argv = %v must NOT carry keep-id (unsupported)", rep.Argv)
	}
}

// TestEngineUnitEmitEmitsExecStartPre pins that exec_start_pre (a declared
// envelope field) reaches the rendered unit as ExecStartPre= directives.
func TestEngineUnitEmitEmitsExecStartPre(t *testing.T) {
	out, err := InvokeEngineOp("nerdctl", ops.OpEngineUnitEmit,
		json.RawMessage(`{"name":"svc","run_mode":"systemd-unit","start_argv":["/usr/bin/nerdctl","run","svc"],"exec_start_pre":["/usr/bin/nerdctl rm -f svc","/bin/true"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var rep spec.EngineUnitReply
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	text := rep.Files["charly-svc.service"]
	if !containsSub(text, "ExecStartPre=/usr/bin/nerdctl rm -f svc") {
		t.Errorf("unit text missing the first ExecStartPre:\n%s", text)
	}
	if !containsSub(text, "ExecStartPre=/bin/true") {
		t.Errorf("unit text missing the second ExecStartPre:\n%s", text)
	}
}

// TestEngineUnitEmitHonorsRequestRunMode pins that the op's decision input is the
// REQUEST's run_mode (the envelope field), not the provider's capability: a
// quadlet request to ANY provider — even nerdctl, whose capability mode is
// systemd-unit — returns no files. This is the doc/servant agreement gate.
func TestEngineUnitEmitHonorsRequestRunMode(t *testing.T) {
	for _, engine := range spec.EngineNames {
		out, err := InvokeEngineOp(engine, ops.OpEngineUnitEmit,
			json.RawMessage(`{"name":"svc","run_mode":"quadlet","start_argv":["x","run"]}`))
		if err != nil {
			t.Fatalf("unit_emit(%s, quadlet): %v", engine, err)
		}
		var rep spec.EngineUnitReply
		if err := json.Unmarshal(out, &rep); err != nil {
			t.Fatal(err)
		}
		if len(rep.Files) != 0 {
			t.Errorf("unit_emit(%s, run_mode=quadlet) = %v, want no files (quadlet req is not this op's)", engine, rep.Files)
		}
	}
}

// TestEngineUnitEmitOp pins the persistence unit: nerdctl's systemd-unit form is
// rendered; podman's quadlet returns no files (podman's own generator emits it).
func TestEngineUnitEmitOp(t *testing.T) {
	out, err := InvokeEngineOp("nerdctl", ops.OpEngineUnitEmit,
		json.RawMessage(`{"name":"svc","run_mode":"systemd-unit","start_argv":["/usr/bin/nerdctl","run","-d","--name","svc","alpine"],"stop_argv":["/usr/bin/nerdctl","stop","svc"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var rep spec.EngineUnitReply
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	text, ok := rep.Files["charly-svc.service"]
	if !ok {
		t.Fatalf("unit_emit(nerdctl) files = %v, want charly-svc.service", rep.Files)
	}
	if !containsSub(text, "ExecStart=/usr/bin/nerdctl run -d --name svc alpine") {
		t.Errorf("unit text missing ExecStart:\n%s", text)
	}

	// podman quadlet → the servant produces no file (podman's generator owns it).
	out, err = InvokeEngineOp("podman", ops.OpEngineUnitEmit,
		json.RawMessage(`{"name":"svc","run_mode":"quadlet"}`))
	if err != nil {
		t.Fatal(err)
	}
	var quadletRep spec.EngineUnitReply
	if err := json.Unmarshal(out, &quadletRep); err != nil {
		t.Fatal(err)
	}
	if len(quadletRep.Files) != 0 {
		t.Errorf("unit_emit(podman quadlet) = %v, want no files (quadlet generator owns it)", quadletRep.Files)
	}

	// A missing name is an error field, not a panic.
	out, err = InvokeEngineOp("nerdctl", ops.OpEngineUnitEmit, json.RawMessage(`{"run_mode":"systemd-unit","start_argv":["x"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var errRep spec.EngineUnitReply
	if err := json.Unmarshal(out, &errRep); err != nil {
		t.Fatal(err)
	}
	if errRep.Error == "" {
		t.Error("unit_emit with no name must set the error field")
	}
}

// TestEngineNetworkEnsureOp pins the network argv.
func TestEngineNetworkEnsureOp(t *testing.T) {
	out, err := InvokeEngineOp("nerdctl", ops.OpEngineNetworkEnsure, json.RawMessage(`{"name":"charly"}`))
	if err != nil {
		t.Fatal(err)
	}
	var rep spec.EngineNetworkEnsureReply
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Network != "charly" || len(rep.Argv) < 4 || rep.Argv[0] != "nerdctl" {
		t.Errorf("network_ensure(nerdctl) = %+v, want network=charly argv starting nerdctl network create", rep)
	}
}

func containsArg(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}

func containsArgPair(argv []string, flag, val string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == val {
			return true
		}
	}
	return false
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestEnginePredicatesRouteThroughTheClassOp proves the class is genuinely
// CONSUMED in-tree: EngineBinary and GPURunArgs are the typed accessors of the
// binary/gpu_args ops, so their answers equal the op replies. This is the gate
// that the class is not a shipped-but-unconsumed contract.
func TestEnginePredicatesRouteThroughTheClassOp(t *testing.T) {
	for _, engine := range append(append([]string{}, spec.EngineNames...), "") {
		out, err := InvokeEngineOp(engine, ops.OpEngineBinary, nil)
		if err != nil {
			t.Fatalf("binary op(%q): %v", engine, err)
		}
		var brep spec.EngineBinaryReply
		if err := json.Unmarshal(out, &brep); err != nil {
			t.Fatal(err)
		}
		if got, want := EngineBinary(engine), brep.Binary; got != want {
			t.Errorf("EngineBinary(%q) = %q, op reply = %q — the accessor must route through the op", engine, got, want)
		}

		out, err = InvokeEngineOp(engine, ops.OpEngineGPURunArgs, nil)
		if err != nil {
			t.Fatalf("gpu_args op(%q): %v", engine, err)
		}
		var grep spec.EngineGPURunArgsReply
		if err := json.Unmarshal(out, &grep); err != nil {
			t.Fatal(err)
		}
		if got, want := GPURunArgs(engine), grep.Args; !reflect.DeepEqual(got, want) {
			t.Errorf("GPURunArgs(%q) = %v, op reply = %v — the accessor must route through the op", engine, got, want)
		}
	}
}
