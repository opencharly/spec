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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/opencharly/spec/ops"
	"github.com/opencharly/spec/spec"
)

// EngineBinary returns the CLI binary name for a container engine. "auto" resolves via
// DetectEngine (podman preferred); an unknown/empty word falls back to the ONE
// default engine (spec.DefaultContainerEngine, podman) so a caller that names no
// engine gets the same engine the exec hop and DetectEngine use. A caller can
// distinguish the fallback from a real word via IsEngineName.
//
// The behavior is served by the engine provider CLASS (the `binary` op); this
// predicate is its typed accessor, so the class has an in-tree consumer and the
// compiled-in / out-of-process placements answer identically.
func EngineBinary(engine string) string {
	if engine == "auto" {
		if detected, err := DetectEngine(); err == nil {
			return detected
		}
		return spec.DefaultContainerEngine
	}
	if out, err := InvokeEngineOp(engine, ops.OpEngineBinary, nil); err == nil {
		var rep spec.EngineBinaryReply
		if json.Unmarshal(out, &rep) == nil {
			return rep.Binary
		}
	}
	return spec.DefaultContainerEngine
}

// GPURunArgs returns the engine-specific run flags that expose all host GPUs to a container.
// podman uses the CDI device form; docker and nerdctl use the Docker-compatible `--gpus`.
// The style rides on the capability table so it is a fact, not a switch. The `"auto"`
// selector resolves to the installed engine via DetectEngine (the same resolution
// EngineBinary performs), so GPURunArgs("auto") matches GPURunArgs(<detected>).
//
// Served by the class's `gpu_args` op; this is its typed accessor. The flag form
// is defined once, in engineGPUArgsRaw (the op body's leaf). The marshal path
// cannot fail for a plain string slice, but if it ever did this falls back to that
// SAME leaf — never a second copy of the flag form.
func GPURunArgs(engine string) []string {
	if engine == "auto" {
		if detected, err := DetectEngine(); err == nil {
			engine = detected
		} else {
			engine = spec.DefaultContainerEngine
		}
	}
	if out, err := InvokeEngineOp(engine, ops.OpEngineGPURunArgs, nil); err == nil {
		var rep spec.EngineGPURunArgsReply
		if json.Unmarshal(out, &rep) == nil && rep.Args != nil {
			return rep.Args
		}
	}
	return engineGPUArgsRaw(engine)
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
		Name:                 "nerdctl",
		Binary:               "nerdctl",
		DetectProbe:          "nerdctl --version",
		SupportsRootless:     true,
		RunMode:              "systemd-unit",
		GPUArgStyle:          "gpus",
		WorkloadUser:         "0",
		ImageExistsArgv:      []string{"image", "inspect"},
		NoRemoveWithDetach:   true,
	},
}

// EngineCapabilityFor returns the capability facts for an engine. There is NO
// distinct "unknown engine": the empty word AND any word that is not a member of
// the closed engine vocabulary both mean "the default engine"
// (spec.DefaultContainerEngine) — an unrecognized word can only be a typo or a
// pre-resolution artifact, and treating it as a separate engine would split one
// input across two engines' facts (podman's binary with docker's gpu args/probe/
// mode, the regression this resolution closes). "auto" resolves through
// DetectEngine first; the bool is false ONLY when that detection fails.
//
// A caller that needs to know whether a word is a REAL engine uses IsEngineName,
// not this bool — ValidateEngine is the authoring-time typo gate.
func EngineCapabilityFor(engine string) (spec.EngineCapability, bool) {
	if engine == "auto" {
		if detected, err := DetectEngine(); err == nil {
			engine = detected
		} else {
			return spec.EngineCapability{}, false
		}
	}
	if _, ok := engineCapabilities[engine]; !ok {
		// "" and every unrecognized word resolve to the ONE default engine, so
		// binary/mode/gpu-args/probe all agree on a single engine.
		engine = spec.DefaultContainerEngine
	}
	c, ok := engineCapabilities[engine]
	return c, ok
}

// EngineRunModeFor returns the persistence/supervision mode for an engine word.
// An EMPTY word AND an unknown non-empty word both resolve to the ONE default
// engine (spec.DefaultContainerEngine) and return its mode — the same word
// EngineBinary resolves them to — so no input has anything but a single
// (binary, mode) resolution. The DirectRunMode() fallback is reached ONLY for
// "auto" when no engine is installed (the conservative, no-unit degraded path).
func EngineRunModeFor(engine string) string {
	if c, ok := EngineCapabilityFor(engine); ok {
		return string(c.RunMode)
	}
	// Unreachable for "" or an unknown word (both resolve to the default engine's
	// capability); reachable only for "auto" when NO engine is installed, where the
	// conservative no-unit path is the right degraded answer.
	return DirectRunMode()
}

// EngineValidationError builds the canonical "not an engine" error text from the
// CUE-owned vocabulary, so the message can never drift from the accepted set.
func EngineValidationError(field, value string) error {
	return fmt.Errorf("%s must be one of %s, got %q", field, strings.Join(spec.EngineNames, ", "), value)
}
