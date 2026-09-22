package container

import (
	"reflect"
	"testing"

	"github.com/opencharly/spec/spec"
)

func TestEngineBinary(t *testing.T) {
	cases := map[string]string{
		"podman":  "podman",
		"docker":  "docker",
		"nerdctl": "nerdctl",
		// unknown/empty falls back to the ONE default engine (spec.DefaultContainerEngine)
		"":      spec.DefaultContainerEngine,
		"bogus": spec.DefaultContainerEngine,
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
	// `"auto"` is the pre-resolution selector: it resolves to the installed engine
	// (DetectEngine), so it must yield the SAME form as that engine — not the
	// historical `--gpus all` default. On a podman host that is the CDI form; the
	// assertion is written against the detected engine so it holds on any host.
	if detected, err := DetectEngine(); err == nil {
		if got, want := GPURunArgs("auto"), GPURunArgs(detected); !reflect.DeepEqual(got, want) {
			t.Errorf("GPURunArgs(auto) = %v, want the detected engine %q's %v", got, detected, want)
		}
	}
}

// TestEngineVocabularyIsSingleSource proves the word list comes from CUE
// (spec.EngineNames) and that IsEngineName / EngineBinary agree with it — so no
// literal switch can drift from the schema.
func TestEngineVocabularyIsSingleSource(t *testing.T) {
	if len(spec.EngineNames) == 0 {
		t.Fatal("spec.EngineNames is empty — the CUE vocabulary did not reach the generated file")
	}
	for _, name := range spec.EngineNames {
		if !IsEngineName(name) {
			t.Errorf("IsEngineName(%q) = false for a CUE-declared engine", name)
		}
		if EngineBinary(name) != name {
			t.Errorf("EngineBinary(%q) = %q, want the engine's own name", name, EngineBinary(name))
		}
		if _, ok := EngineCapabilityFor(name); !ok {
			t.Errorf("engine %q has no capability row", name)
		}
	}
	// "auto" and typos are not engine WORDS.
	if IsEngineName("auto") {
		t.Error(`IsEngineName("auto") must be false — auto is a selector, not an engine word`)
	}
	if IsEngineName("bogus") {
		t.Error(`IsEngineName("bogus") must be false`)
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
	if podman.Name != "podman" {
		t.Errorf("podman Name = %q, want podman", podman.Name)
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
	if nerdctl.DetectProbe == "" {
		t.Error("nerdctl must declare a detect_probe")
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

func TestIsRunMode(t *testing.T) {
	for _, m := range spec.EngineRunModes {
		if !IsRunMode(m) {
			t.Errorf("IsRunMode(%q) = false for a CUE-declared run mode", m)
		}
	}
	// "auto" is the pre-resolution selector, not a run-mode word.
	if IsRunMode("auto") {
		t.Error(`IsRunMode("auto") must be false — auto is a selector`)
	}
	if IsRunMode("bogus") {
		t.Error(`IsRunMode("bogus") must be false`)
	}
}

// TestEngineCapabilityShapeMatchesSchema is the drift gate: the table uses the
// generated spec.EngineCapability type (not a hand mirror), so every row's
// RunMode must be a member of the CUE-owned run-mode vocabulary.
func TestEngineCapabilityShapeMatchesSchema(t *testing.T) {
	for name, c := range engineCapabilities {
		if string(c.Name) != name {
			t.Errorf("capability row %q has Name %q", name, c.Name)
		}
		if !IsEngineName(string(c.Name)) {
			t.Errorf("capability row %q Name is not in spec.EngineNames", name)
		}
		if c.RunMode != "" && !IsRunMode(string(c.RunMode)) {
			t.Errorf("capability row %q RunMode %q is not in spec.EngineRunModes", name, c.RunMode)
		}
	}
}

// TestDirectRunModeDerived proves the non-unit mode name is DERIVED as
// EngineRunModes − EngineUnitRunModeWords, not a second literal: it must be a
// member of the run-mode vocabulary and NOT a unit mode.
func TestDirectRunModeDerived(t *testing.T) {
	got := DirectRunMode()
	if got == "" {
		t.Fatal("DirectRunMode() is empty")
	}
	if !IsRunMode(got) {
		t.Errorf("DirectRunMode() = %q, not a member of spec.EngineRunModes", got)
	}
	if IsUnitRunMode(got) {
		t.Errorf("DirectRunMode() = %q, but it must NOT be unit-supervised", got)
	}
	// Exactly one non-unit mode exists in the CUE vocabulary.
	nonUnit := 0
	for _, m := range spec.EngineRunModes {
		if !IsUnitRunMode(m) {
			nonUnit++
			if m != got {
				t.Errorf("non-unit mode %q != DirectRunMode() %q", m, got)
			}
		}
	}
	if nonUnit != 1 {
		t.Errorf("expected exactly 1 non-unit run mode, found %d", nonUnit)
	}
}
