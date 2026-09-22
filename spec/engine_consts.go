package spec

// engine_consts.go — the container-engine DEFAULT constant, the ONE importable
// home (R3). Two consumers need "the engine to use when none was named": the
// container package's EngineBinary (container/engine.go — container imports exec,
// so exec cannot import it) and the exec package's NestedJump.engineBinary
// (exec/deploy_executor_nested.go). Before this file each carried its own literal
// and they diverged: EngineBinary("") returned "docker" while the exec hop that
// actually runs used "podman", so the same unspecified input resolved to two
// different engines. Both now read DefaultContainerEngine.
//
// It lives in package spec because spec is the only common base (container ->
// exec is one-way, so neither can host it). This file holds ONLY the constant;
// the engine vocabulary and capability facts are CUE-owned (schema/engine.cue).
const (
	// DefaultContainerEngine is the engine used when the caller names none (the
	// empty string). It is podman — the historical default and the engine
	// DetectEngine prefers, so an unspecified deploy resolves to the same engine
	// the auto-detect path would pick.
	DefaultContainerEngine = "podman"
)
