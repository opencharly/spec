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
	for _, engine := range []string{"docker", "nerdctl"} {
		got := GPURunArgs(engine)
		if len(got) != 2 || got[0] != "--gpus" || got[1] != "all" {
			t.Errorf("GPURunArgs(%q) = %v, want --gpus all", engine, got)
		}
	}
	// The EMPTY word means "unspecified" and resolves to the ONE default engine, so
	// its GPU form is the default engine's (podman → CDI), NOT the historical
	// docker-style `--gpus all`. The empty-word resolution must agree with
	// EngineBinary("") — one input, one engine.
	wantEmpty := GPURunArgs(EngineBinary(""))
	if got := GPURunArgs(""); !reflect.DeepEqual(got, wantEmpty) {
		t.Errorf("GPURunArgs(\"\") = %v, want the default engine's %v (EngineBinary(\"\")=%q)",
			got, wantEmpty, EngineBinary(""))
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

// TestUnknownEngineResolutionIsSingleHome proves an UNKNOWN non-empty word does
// not split one input across two engines either: for every consumer,
// f(word) == f(EngineBinary(word)). Before the fix, EngineBinary("bogus") was
// the default (podman) while GPURunArgs("bogus")/imageExistsProbeArgv("bogus")
// took the docker/nerdctl fallback — podman's binary with docker's args/probe.
// IsEngineName is the typo gate; the capability resolution treats any non-member
// as the default engine so the consumers agree.
func TestUnknownEngineResolutionIsSingleHome(t *testing.T) {
	for _, word := range []string{"bogus", "containerd", "Podman", "docker-23"} {
		resolved := EngineBinary(word)
		if got := GPURunArgs(word); !reflect.DeepEqual(got, GPURunArgs(resolved)) {
			t.Errorf("GPURunArgs(%q)=%v != GPURunArgs(EngineBinary(%q))=%v", word, got, word, GPURunArgs(resolved))
		}
		if got := imageExistsProbeArgv(word); !reflect.DeepEqual(got, imageExistsProbeArgv(resolved)) {
			t.Errorf("imageExistsProbeArgv(%q)=%v != imageExistsProbeArgv(EngineBinary(%q))=%v", word, got, word, imageExistsProbeArgv(resolved))
		}
		if got, want := EngineRunModeFor(word), EngineRunModeFor(resolved); got != want {
			t.Errorf("EngineRunModeFor(%q)=%q != EngineRunModeFor(EngineBinary(%q))=%q", word, got, word, want)
		}
		// A real typo is caught by IsEngineName, not by a split resolution.
		if IsEngineName(word) {
			t.Errorf("IsEngineName(%q) must be false — it is not in the closed vocabulary", word)
		}
	}
}

// TestEmptyEngineResolutionIsSingleHome pins the block-1 invariant: for the
// unspecified ("") engine, the binary, run mode, GPU args, and local-image probe
// ALL resolve to the SAME engine — the ONE default (spec.DefaultContainerEngine).
// Before the fix, EngineBinary("") was the default (podman) while GPURunArgs("")
// and imageExistsProbeArgv("") — reading the capability table and getting
// not-found for "" — fell back to docker/nerdctl's forms, so one input split
// across two engines. This is the regression gate for that split.
func TestEmptyEngineResolutionIsSingleHome(t *testing.T) {
	def := spec.DefaultContainerEngine
	// Every consumer's "" resolution must equal its resolution for the default WORD.
	if got, want := EngineBinary(""), EngineBinary(def); got != want {
		t.Errorf("EngineBinary(\"\") = %q, want the default word's %q", got, want)
	}
	if got, want := EngineRunModeFor(""), EngineRunModeFor(def); got != want {
		t.Errorf("EngineRunModeFor(\"\") = %q, want the default word's %q", got, want)
	}
	if got, want := GPURunArgs(""), GPURunArgs(def); !reflect.DeepEqual(got, want) {
		t.Errorf("GPURunArgs(\"\") = %v, want the default word's %v", got, want)
	}
	if got, want := imageExistsProbeArgv(""), imageExistsProbeArgv(def); !reflect.DeepEqual(got, want) {
		t.Errorf("imageExistsProbeArgv(\"\") = %v, want the default word's %v", got, want)
	}
	// And the requested cross-checks against EngineBinary("") directly.
	if got, want := GPURunArgs(""), GPURunArgs(EngineBinary("")); !reflect.DeepEqual(got, want) {
		t.Errorf("GPURunArgs(\"\") = %v, want GPURunArgs(EngineBinary(\"\")) = %v", got, want)
	}
	if got, want := imageExistsProbeArgv(""), imageExistsProbeArgv(EngineBinary("")); !reflect.DeepEqual(got, want) {
		t.Errorf("imageExistsProbeArgv(\"\") = %v, want imageExistsProbeArgv(EngineBinary(\"\")) = %v", got, want)
	}
	// The default engine (podman) carries the CDI GPU form and the `image exists`
	// probe; assert the concrete values so the resolution is not vacuously equal.
	if got := GPURunArgs(""); len(got) != 2 || got[0] != "--device" {
		t.Errorf("GPURunArgs(\"\") = %v, want the default engine's CDI form", got)
	}
	if got := imageExistsProbeArgv(""); !reflect.DeepEqual(got, []string{"image", "exists"}) {
		t.Errorf("imageExistsProbeArgv(\"\") = %v, want the default engine's {image exists}", got)
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
	// nerdctl rejects `-d` with `--rm` (measured live on 2.3.5); podman/docker
	// accept both. The fact lets a detached-argv builder drop --rm where illegal.
	if !nerdctl.NoRemoveWithDetach {
		t.Error("nerdctl must set NoRemoveWithDetach (flags -d and --rm cannot be combined)")
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
	// docker (like podman) accepts `-d --rm`.
	if docker.NoRemoveWithDetach {
		t.Error("docker accepts -d with --rm; NoRemoveWithDetach must be false")
	}
	if podman.NoRemoveWithDetach {
		t.Error("podman accepts -d with --rm; NoRemoveWithDetach must be false")
	}

	// An unknown non-empty word resolves to the ONE default engine (like the
	// empty word), so binary/mode/gpu-args/probe never split across two engines.
	// IsEngineName is the typo gate, not this bool.
	bogus, ok := EngineCapabilityFor("bogus")
	if !ok {
		t.Fatal("EngineCapabilityFor(bogus) must resolve (to the default engine), not report not-found")
	}
	if bogus.Binary != spec.DefaultContainerEngine {
		t.Errorf("EngineCapabilityFor(bogus).Binary = %q, want the default engine %q", bogus.Binary, spec.DefaultContainerEngine)
	}
}

func TestEngineRunModeFor(t *testing.T) {
	if got := EngineRunModeFor("podman"); got != "quadlet" {
		t.Errorf("EngineRunModeFor(podman) = %q, want quadlet", got)
	}
	if got := EngineRunModeFor("nerdctl"); got != "systemd-unit" {
		t.Errorf("EngineRunModeFor(nerdctl) = %q, want systemd-unit", got)
	}
	// An unknown non-empty word == the default engine (podman → quadlet), the
	// same single-home resolution as the empty word — never a third engine's mode.
	if got := EngineRunModeFor("bogus"); got != EngineRunModeFor(spec.DefaultContainerEngine) {
		t.Errorf("EngineRunModeFor(bogus) = %q, want the default engine's mode %q", got, EngineRunModeFor(spec.DefaultContainerEngine))
	}
	// The EMPTY engine means "the default engine", and MUST resolve to the same
	// engine EngineBinary("") resolves to — one (binary, mode) resolution, never
	// podman's binary with docker's mode.
	if got := EngineRunModeFor(""); got != EngineRunModeFor(spec.DefaultContainerEngine) {
		t.Errorf("EngineRunModeFor(\"\") = %q, want the default engine's mode %q (EngineBinary(\"\")=%q)",
			got, EngineRunModeFor(spec.DefaultContainerEngine), EngineBinary(""))
	}
	if got := EngineRunModeFor(""); got != "quadlet" {
		t.Errorf("EngineRunModeFor(\"\") = %q, want quadlet (the default engine spec.DefaultContainerEngine=podman mode)", got)
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
