package container

import "testing"

func TestEngineBinary(t *testing.T) {
	cases := map[string]string{
		"podman":  "podman",
		"docker":  "docker",
		"nerdctl": "nerdctl",
		"":        "docker",
		"bogus":   "docker",
	}
	for engine, want := range cases {
		if got := EngineBinary(engine); got != want {
			t.Errorf("EngineBinary(%q) = %q, want %q", engine, got, want)
		}
	}
}

func TestGPURunArgs(t *testing.T) {
	if got := GPURunArgs("podman"); len(got) != 2 || got[0] != "--device" || got[1] != "nvidia.com/gpu=all" {
		t.Errorf("GPURunArgs(podman) = %v, want CDI device form", got)
	}
	for _, engine := range []string{"docker", "nerdctl", ""} {
		got := GPURunArgs(engine)
		if len(got) != 2 || got[0] != "--gpus" || got[1] != "all" {
			t.Errorf("GPURunArgs(%q) = %v, want --gpus all", engine, got)
		}
	}
}

func TestEngineCapabilityFor(t *testing.T) {
	podman, ok := EngineCapabilityFor("podman")
	if !ok {
		t.Fatal("EngineCapabilityFor(podman) not found")
	}
	if !podman.SupportsPods || !podman.SupportsSecrets || !podman.SupportsUsernsKeepID {
		t.Errorf("podman capability lost a native feature: %+v", podman)
	}
	if podman.RunMode != "quadlet" {
		t.Errorf("podman RunMode = %q, want quadlet", podman.RunMode)
	}
	if podman.GPUArgStyle != "cdi" {
		t.Errorf("podman GPUArgStyle = %q, want cdi", podman.GPUArgStyle)
	}

	nerdctl, ok := EngineCapabilityFor("nerdctl")
	if !ok {
		t.Fatal("EngineCapabilityFor(nerdctl) not found")
	}
	if nerdctl.Binary != "nerdctl" {
		t.Errorf("nerdctl Binary = %q, want nerdctl", nerdctl.Binary)
	}
	if nerdctl.SupportsPods || nerdctl.SupportsSecrets || nerdctl.SupportsUsernsKeepID {
		t.Errorf("nerdctl must not claim a podman-only native feature: %+v", nerdctl)
	}
	if !nerdctl.SupportsRootless {
		t.Error("nerdctl must be rootless-capable")
	}
	if nerdctl.RunMode != "systemd-unit" {
		t.Errorf("nerdctl RunMode = %q, want systemd-unit", nerdctl.RunMode)
	}
	if nerdctl.GPUArgStyle != "gpus" {
		t.Errorf("nerdctl GPUArgStyle = %q, want gpus", nerdctl.GPUArgStyle)
	}
	// The spike-proven sharing invariant: no keep-id means the workload must run
	// as container-uid 0, which IS the invoking host user in the rootless userns.
	if nerdctl.WorkloadUser != "0" {
		t.Errorf("nerdctl WorkloadUser = %q, want 0 (uid 0 == host user rootless)", nerdctl.WorkloadUser)
	}

	docker, ok := EngineCapabilityFor("docker")
	if !ok {
		t.Fatal("EngineCapabilityFor(docker) not found")
	}
	if docker.RunMode != "direct" {
		t.Errorf("docker RunMode = %q, want direct", docker.RunMode)
	}
	if docker.SupportsSecrets {
		t.Error("docker must not claim a native secret store")
	}

	if _, ok := EngineCapabilityFor("bogus"); ok {
		t.Error("EngineCapabilityFor(bogus) must report not-found")
	}
}

func TestEngineRunModeFor(t *testing.T) {
	if got := EngineRunModeFor("podman"); got != "quadlet" {
		t.Errorf("EngineRunModeFor(podman) = %q, want quadlet", got)
	}
	if got := EngineRunModeFor("nerdctl"); got != "systemd-unit" {
		t.Errorf("EngineRunModeFor(nerdctl) = %q, want systemd-unit", got)
	}
	if got := EngineRunModeFor("bogus"); got != "direct" {
		t.Errorf("EngineRunModeFor(bogus) = %q, want direct (conservative default)", got)
	}
}

// TestEveryEngineHasCapability is the drift gate: the engine words in
// EngineBinary and the capability table must never disagree, so adding an
// engine to one place without the other fails here rather than at deploy time.
func TestEveryEngineHasCapability(t *testing.T) {
	for _, engine := range []string{"podman", "docker", "nerdctl"} {
		c, ok := EngineCapabilityFor(engine)
		if !ok {
			t.Errorf("engine %q has no capability row", engine)
			continue
		}
		if c.Binary != EngineBinary(engine) {
			t.Errorf("engine %q: capability Binary %q != EngineBinary %q", engine, c.Binary, EngineBinary(engine))
		}
	}
}
