// Package container is the spec fabric slice for container-engine host helpers — resolving
// which engine binary to invoke, its GPU run-args, and auto-detecting the installed engine.
// RELOCATED from sdk/kit (#55 fabric-primitive extraction). It carries os/exec (the engine
// auto-detect shells `LookPath`) in its OWN slice (Rule 2) so a consumer needing only value
// types never drags os/exec. charly core inlines from here; sdk/kit re-exports the same symbols
// so existing kit.EngineBinary / kit.GPURunArgs / kit.DetectEngine call sites are untouched.
package container

import (
	"fmt"
	"os/exec"
)

// EngineBinary returns the CLI binary name for a container engine. "auto" resolves via
// DetectEngine (podman preferred), falling back to "docker".
func EngineBinary(engine string) string {
	switch engine {
	case "podman":
		return "podman"
	case "docker":
		return "docker"
	case "nerdctl":
		return "nerdctl"
	case "auto":
		if detected, err := DetectEngine(); err == nil {
			return detected
		}
		return "docker"
	default:
		return "docker"
	}
}

// GPURunArgs returns the engine-specific run flags that expose all host GPUs to a container.
// podman uses the CDI device form; docker and nerdctl use the Docker-compatible `--gpus`.
func GPURunArgs(engine string) []string {
	switch engine {
	case "podman":
		return []string{"--device", "nvidia.com/gpu=all"}
	default:
		return []string{"--gpus", "all"}
	}
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

// EngineCapability is the static, data-only description of an engine: the facts
// every "does this engine support X" branch in core consults instead of
// switching on the engine name. It mirrors the authored #EngineCapability CUE
// def (schema/engine.cue); the provider contract carries the same shape on the
// wire. Kept as a plain Go table here because the name->facts mapping is DATA
// (like EngineBinary), available before any engine provider connects.
type EngineCapability struct {
	// Binary is the CLI binary the engine is driven through.
	Binary string
	// SupportsPods is true when the engine has a first-class pod primitive
	// (podman's .pod). False means pod-style sharing uses a shared network
	// namespace (--net=container:<primary>).
	SupportsPods bool
	// SupportsSecrets is true when the engine has a native secret store
	// (podman secret). False means credentials are delivered as env/file.
	SupportsSecrets bool
	// SupportsUsernsKeepID is true when the engine can map the invoking user
	// into the container (podman --userns=keep-id). False means host-identical
	// file sharing requires launching the workload as container-uid-0 under a
	// rootless single-userns engine (nerdctl).
	SupportsUsernsKeepID bool
	// SupportsRootless is true when the engine runs without host root.
	SupportsRootless bool
	// RunMode is the persistence/supervision mode: "quadlet" (podman),
	// "systemd-unit" (nerdctl; a generated .service wrapping the CLI), or
	// "direct" (docker; ephemeral argv).
	RunMode string
	// GPUArgStyle is "cdi" (podman --device nvidia.com/gpu=all) or "gpus"
	// (docker/nerdctl --gpus all).
	GPUArgStyle string
	// UsernsKeepIDArg is the per-container keep-id argv when supported.
	UsernsKeepIDArg string
	// WorkloadUser is the in-container user a workload must run as for
	// host-identical file sharing. "0" for rootless single-userns engines
	// (container-uid 0 IS the invoking host user in the rootless userns);
	// empty means the engine's keep-id mapping handles it.
	WorkloadUser string
}

// engineCapabilities is the DATA table the kernel consults. One row per engine
// word; the values are facts about the engine CLI, not policy. nerdctl's row is
// the spike-proven posture: rootless, no pods primitive (shared-netns
// emulation), no native secret store (env/file fallback), no keep-id
// (workload runs as uid 0 == invoking user), systemd-unit persistence, Docker's
// `--gpus` passthrough.
var engineCapabilities = map[string]EngineCapability{
	"podman": {
		Binary:               "podman",
		SupportsPods:         true,
		SupportsSecrets:      true,
		SupportsUsernsKeepID: true,
		SupportsRootless:     true,
		RunMode:              "quadlet",
		GPUArgStyle:          "cdi",
		UsernsKeepIDArg:      "--userns=keep-id",
	},
	"docker": {
		Binary:               "docker",
		SupportsPods:         false,
		SupportsSecrets:      false,
		SupportsUsernsKeepID: false,
		SupportsRootless:     true,
		RunMode:              "direct",
		GPUArgStyle:          "gpus",
	},
	"nerdctl": {
		Binary:               "nerdctl",
		SupportsPods:         false,
		SupportsSecrets:      false,
		SupportsUsernsKeepID: false,
		SupportsRootless:     true,
		RunMode:              "systemd-unit",
		GPUArgStyle:          "gpus",
		WorkloadUser:         "0",
	},
}

// EngineCapabilityFor returns the capability facts for an engine word. The
// bool is false for an unknown/empty word, so a caller can distinguish "docker"
// (a known engine with known limitations) from a typo. "auto" resolves through
// DetectEngine first so a caller never has to.
func EngineCapabilityFor(engine string) (EngineCapability, bool) {
	if engine == "auto" {
		if detected, err := DetectEngine(); err == nil {
			engine = detected
		} else {
			return EngineCapability{}, false
		}
	}
	c, ok := engineCapabilities[engine]
	return c, ok
}

// EngineRunModeFor returns the persistence/supervision mode for an engine word,
// defaulting to "direct" for an unknown word (the conservative, no-unit path).
func EngineRunModeFor(engine string) string {
	if c, ok := EngineCapabilityFor(engine); ok {
		return c.RunMode
	}
	return "direct"
}
