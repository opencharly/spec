package hostenv

import (
	"os"
	"path/filepath"
	"testing"
)

// The image BUILDER is BuildKit, which is not a container engine: it cannot run a
// container, so it can never be a member of the closed engine vocabulary
// (#EngineName → spec.EngineNames). The contract therefore carries no builder
// selector at all — `engine.build` / CHARLY_BUILD_ENGINE / ResolvedRuntime.BuildEngine
// are gone, and every consumer that read them now reads the RUN engine.
//
// The ABSENCE of those names is asserted by the contract's list-driven guard
// (`retired.Identifiers`): the canonical retired-identifier list holds the withdrawn
// names and one guard reads that list, so the assertion lives in one place and a
// withdrawn name is never restated per type.
//
// The documented AFTERMATH of the cutover, guarded so the promise in the change's
// own notes cannot silently stop being true: an existing `engine.build:` in
// ~/.config/charly/config.yml is INERT, not a load failure. RuntimeConfig is parsed
// with a plain yaml.Unmarshal (no closed gate — the closedness gate is the
// charly.yml ingress), so a retired key is ignored and every other key still
// resolves.
//
// Note the honest scope: this is a regression guard for that guarantee, NOT the
// fail-without coverage of the withdrawal itself — the list-driven guard is that.
// It can still fail (a future closed gate or hard error on the retired key trips it).
func TestResolveRuntime_RetiredBuildKeyIsInert(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	// Exactly what a user who ran `charly settings set engine.build docker` has on disk.
	if err := os.WriteFile(path, []byte("engine:\n  build: docker\n  run: podman\n"), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}
	orig := RuntimeConfigPath
	RuntimeConfigPath = func() (string, error) { return path, nil }
	defer func() { RuntimeConfigPath = orig }()

	// Deterministic, ambient-free resolution: an EMPTY env var falls through to the
	// config file (ResolveValue: env != "" wins, else cfg, else default).
	t.Setenv("CHARLY_RUN_ENGINE", "")
	t.Setenv("CHARLY_RUN_MODE", "direct")

	rt, err := ResolveRuntime()
	if err != nil {
		t.Fatalf("a retired engine.build must not fail the load: %v", err)
	}
	if rt.RunEngine != "podman" {
		t.Errorf("RunEngine = %q, want %q — the run key must still resolve beside a retired build key", rt.RunEngine, "podman")
	}
}
