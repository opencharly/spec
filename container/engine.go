// Package container is the spec fabric slice for container-engine host helpers — resolving
// which engine binary to invoke, its GPU run-args, auto-detecting the installed engine, and the
// engine capability data table. RELOCATED from sdk/kit (#55 fabric-primitive extraction). It
// carries os/exec (the engine auto-detect shells `LookPath`) in its OWN slice (Rule 2) so a
// consumer needing only value types never drags os/exec. charly core inlines from here; sdk/kit
// re-exports the same symbols so existing kit.EngineBinary / kit.GPURunArgs / kit.DetectEngine
// call sites are untouched.
//
// The engine vocabularies are CUE-owned (schema/engine.cue #EngineName /
// #EngineRunMode / #EngineUnitRunModes, emitted as spec.EngineNames /
// spec.EngineRunModes / spec.EngineUnitRunModeWords) — this file declares no
// separate word LIST. Each predicate reads its OWN emitted list: IsEngineName →
// spec.EngineNames, IsRunMode → spec.EngineRunModes, IsUnitRunMode →
// spec.EngineUnitRunModeWords. The capability FACTS live in one Go table whose
// KEYS are the engine words (they appear as data, not as a second list); adding
// an engine is one CUE edit plus one table row, gated by
// TestEngineVocabularyIsSingleSource.
package container

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/opencharly/spec/spec"
)

// EngineBinary returns the CLI binary name for a container engine. "auto" resolves via
// DetectEngine (podman preferred); an unknown/empty word falls back to the ONE
// default engine (spec.DefaultContainerEngine, podman) so a caller that names no
// engine gets the same engine the exec hop and DetectEngine use. A caller can
// distinguish the fallback from a real word via IsEngineName.
func EngineBinary(engine string) string {
	if engine == "auto" {
		if detected, err := DetectEngine(); err == nil {
			return detected
		}
		return spec.DefaultContainerEngine
	}
	if c, ok := engineCapabilities[engine]; ok {
		return c.Binary
	}
	return spec.DefaultContainerEngine
}

// GPURunArgs returns the engine-specific run flags that expose all host GPUs to a container.
// podman uses the CDI device form; docker and nerdctl use the Docker-compatible `--gpus`.
// The style rides on the capability table so it is a fact, not a switch. The `"auto"`
// selector resolves to the installed engine via DetectEngine (the same resolution
// EngineBinary performs), so GPURunArgs("auto") matches GPURunArgs(<detected>).
func GPURunArgs(engine string) []string {
	if c, ok := EngineCapabilityFor(engine); ok && c.GPUArgStyle == "cdi" {
		return []string{"--device", "nvidia.com/gpu=all"}
	}
	return []string{"--gpus", "all"}
}

// DetectEngine auto-detects the container engine: prefers podman, falls back to docker.
// nerdctl is deliberately NOT auto-detected — it is opt-in per deploy/box via the
// `engine:` field, so an existing podman/docker install is never silently switched.
func DetectEngine() (string, error) {
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman", nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", nil
	}
	return "", fmt.Errorf("no container engine found (install podman or docker)")
}

// EngineNames returns the closed engine word vocabulary (podman/docker/nerdctl),
// sourced from the CUE-owned schema (schema/engine.cue #EngineName →
// spec.EngineNames) so no consumer holds a hand-written literal list.
func EngineNames() []string { return spec.EngineNames }

// IsEngineName reports whether name is a member of the closed engine vocabulary.
// It does NOT resolve "auto" (that is a selector, not an engine word) — callers
// validating authored config reject "auto" here and resolve it separately.
func IsEngineName(name string) bool { return contains(spec.EngineNames, name) }

// RunModes returns the closed run-mode vocabulary (quadlet/systemd-unit/direct).
func RunModes() []string { return spec.EngineRunModes }

// IsRunMode reports whether mode is a member of the closed run-mode vocabulary.
func IsRunMode(mode string) bool { return contains(spec.EngineRunModes, mode) }

// IsUnitRunMode reports whether mode is supervised by a generated unit file
// (quadlet / systemd-unit) rather than an ephemeral argv launch (direct). The
// unit-capable subset is CUE-owned (#EngineUnitRunModes → spec.EngineUnitRunModeWords),
// so no caller hand-lists {"quadlet","systemd-unit"}.
func IsUnitRunMode(mode string) bool { return contains(spec.EngineUnitRunModeWords, mode) }

// DirectRunMode returns the non-unit run mode — the one member of the CUE-owned
// spec.EngineRunModes that is NOT unit-supervised. Derived as the set difference
// (EngineRunModes − EngineUnitRunModeWords), so the name has NO literal home:
// changing #EngineRunMode is the only edit. Returns "" if the vocabulary is
// malformed (no non-unit mode); TestDirectRunModeDerived pins the real vocabulary
// has exactly one.
func DirectRunMode() string {
	for _, m := range spec.EngineRunModes {
		if !IsUnitRunMode(m) {
			return m
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// engineCapabilities is the DATA table the kernel consults: one row per engine
// word, using the generated spec.EngineCapability shape (never a hand mirror).
// The values are facts about each engine CLI, not policy — they replace every
// "does this engine support X" branch in core. nerdctl's row is the spike-proven
// posture: rootless, no pods primitive (shared-netns emulation), no native
// secret store (env/file fallback), no keep-id (workload runs as uid 0 == the
// invoking host user), systemd-unit persistence, Docker `--gpus` passthrough.
var engineCapabilities = map[string]spec.EngineCapability{
	"podman": {
		Name:                 "podman",
		Binary:               "podman",
		DetectProbe:          "podman --version",
		SupportsPods:         true,
		SupportsSecrets:      true,
		SupportsUsernsKeepID: true,
		SupportsRootless:     true,
		RunMode:              "quadlet",
		GPUArgStyle:          "cdi",
		UsernsKeepIDArg:      "--userns=keep-id",
		ImageExistsArgv:      []string{"image", "exists"},
	},
	"docker": {
		Name:             "docker",
		Binary:           "docker",
		DetectProbe:      "docker --version",
		SupportsRootless: true,
		RunMode:          "direct",
		GPUArgStyle:      "gpus",
		ImageExistsArgv:  []string{"image", "inspect"},
	},
	"nerdctl": {
		Name:             "nerdctl",
		Binary:           "nerdctl",
		DetectProbe:      "nerdctl --version",
		SupportsRootless: true,
		RunMode:          "systemd-unit",
		GPUArgStyle:      "gpus",
		WorkloadUser:     "0",
		ImageExistsArgv:  []string{"image", "inspect"},
	},
}

// EngineCapabilityFor returns the capability facts for an engine word. The bool
// is false for an unknown/empty word, so a caller can distinguish "docker" (a
// known engine with known limitations) from a typo. "auto" resolves through
// DetectEngine first so a caller never has to.
func EngineCapabilityFor(engine string) (spec.EngineCapability, bool) {
	if engine == "auto" {
		if detected, err := DetectEngine(); err == nil {
			engine = detected
		} else {
			return spec.EngineCapability{}, false
		}
	}
	c, ok := engineCapabilities[engine]
	return c, ok
}

// EngineRunModeFor returns the persistence/supervision mode for an engine word.
// An EMPTY word means "the default engine" and resolves to its mode — the same
// word EngineBinary("") resolves to — so an unspecified engine has ONE
// (binary, mode) resolution. A genuinely unknown NON-EMPTY word falls back to
// the non-unit mode (the conservative, no-unit path; the name is derived via
// DirectRunMode, never a literal).
func EngineRunModeFor(engine string) string {
	if engine == "" {
		engine = spec.DefaultContainerEngine
	}
	if c, ok := EngineCapabilityFor(engine); ok {
		return string(c.RunMode)
	}
	return DirectRunMode()
}

// EngineValidationError builds the canonical "not an engine" error text from the
// CUE-owned vocabulary, so the message can never drift from the accepted set.
func EngineValidationError(field, value string) error {
	return fmt.Errorf("%s must be one of %s, got %q", field, strings.Join(spec.EngineNames, ", "), value)
}
