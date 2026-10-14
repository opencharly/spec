package hostenv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The image BUILDER is BuildKit, which is not a container engine: it cannot run a
// container, so it can never be a member of the closed engine vocabulary
// (#EngineName → spec.EngineNames). The contract therefore carries no builder
// selector at all — `engine.build` / CHARLY_BUILD_ENGINE / ResolvedRuntime.BuildEngine
// are gone, and every consumer that read them now reads the RUN engine.
//
// This is the cutover's fail-without instrument for the Go half: it FAILS while
// those fields exist, which is what keeps the removal from being re-added by a
// later "compatibility" commit.
func TestNoBuilderSelectorInTheContract(t *testing.T) {
	// POSITIVE CONTROL first: the reflective instrument can find a field that IS
	// there, so the two negative lookups below cannot pass vacuously.
	if _, ok := reflect.TypeOf(ResolvedRuntime{}).FieldByName("RunEngine"); !ok {
		t.Fatal("control failed: ResolvedRuntime.RunEngine not found — the instrument is broken, not the contract")
	}
	if _, ok := reflect.TypeOf(EngineConfig{}).FieldByName("Run"); !ok {
		t.Fatal("control failed: EngineConfig.Run not found — the instrument is broken, not the contract")
	}

	if f, ok := reflect.TypeOf(EngineConfig{}).FieldByName("Build"); ok {
		t.Errorf("EngineConfig must not carry a builder selector; found field %q (yaml %q)", f.Name, f.Tag.Get("yaml"))
	}
	if f, ok := reflect.TypeOf(ResolvedRuntime{}).FieldByName("BuildEngine"); ok {
		t.Errorf("ResolvedRuntime must not carry a build engine; found field %q", f.Name)
	}
}

// The documented AFTERMATH of the cutover, guarded so the promise in the change's
// own notes cannot silently stop being true: an existing `engine.build:` in
// ~/.config/charly/config.yml is INERT, not a load failure. RuntimeConfig is parsed
// with a plain yaml.Unmarshal (no closed gate — the closedness gate is the
// charly.yml ingress), so a retired key is ignored and every other key still
// resolves.
//
// Note the honest scope: this is a regression guard for that guarantee, NOT this
// change's fail-without coverage — TestNoBuilderSelectorInTheContract above is that.
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
