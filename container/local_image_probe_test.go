package container

import (
	"reflect"
	"testing"
)

// TestImageExistsProbeArgv is the coverage for the capability-driven local-store
// probe. It fails without the change: before, the engine→subcommand mapping was
// a `switch engine { case "podman" … }` inside defaultLocalImageExists; now it
// is the capability table's ImageExistsArgv.
func TestImageExistsProbeArgv(t *testing.T) {
	cases := map[string][]string{
		// podman has a dedicated `image exists` subcommand.
		"podman": {"image", "exists"},
		// docker has no `image exists`; docker and nerdctl both use `image inspect`.
		"docker":  {"image", "inspect"},
		"nerdctl": {"image", "inspect"},
		// The EMPTY word AND an unknown non-empty word both resolve to the ONE
		// default engine (podman → `image exists`), agreeing with EngineBinary, so
		// one input never splits across two engines' probes.
		"":      {"image", "exists"},
		"bogus": {"image", "exists"},
	}
	for engine, want := range cases {
		if got := imageExistsProbeArgv(engine); !reflect.DeepEqual(got, want) {
			t.Errorf("imageExistsProbeArgv(%q) = %v, want %v", engine, got, want)
		}
	}
}

// TestImageExistsProbeArgvMatchesTable ties the probe to the capability table,
// so a new engine's row supplies its own probe rather than silently inheriting
// the inspect fallback.
func TestImageExistsProbeArgvMatchesTable(t *testing.T) {
	for _, name := range EngineNames() {
		c, ok := EngineCapabilityFor(name)
		if !ok {
			t.Errorf("engine %q has no capability row", name)
			continue
		}
		if len(c.ImageExistsArgv) == 0 {
			t.Errorf("engine %q capability row has no ImageExistsArgv", name)
		}
		if got := imageExistsProbeArgv(name); !reflect.DeepEqual(got, c.ImageExistsArgv) {
			t.Errorf("imageExistsProbeArgv(%q) = %v, want the table's %v", name, got, c.ImageExistsArgv)
		}
	}
}

// TestResolveShellImageRefProbesInstalledEngine gates the fix for the local
// CalVer resolution: ResolveShellImageRef must resolve against the engine
// ACTUALLY installed on the host, not the hardcoded "podman". Before the change
// it called ResolveNewestLocalCalVer("podman", name) — wrong on a docker/nerdctl
// host, where it inspected the wrong local store. The engine that reaches the
// lister must therefore be the "auto" selector (which EngineBinary resolves via
// DetectEngine), not a literal engine word. This test FAILS if the call reverts
// to "podman".
func TestResolveShellImageRefProbesInstalledEngine(t *testing.T) {
	origList := ListLocalImages
	var gotEngine string
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		gotEngine = engine
		return nil, nil // no local match: the resolver falls back to the bare ref
	}
	t.Cleanup(func() { ListLocalImages = origList })

	// Best-effort: no local image, so this returns the tagless fallback.
	_ = ResolveShellImageRef("", "somebox", "")

	if gotEngine != "auto" {
		t.Fatalf("ResolveShellImageRef probed engine %q, want the \"auto\" selector "+
			"(a literal engine word assumes the host's engine)", gotEngine)
	}
	// And the selector must resolve to whatever is installed here.
	if detected, err := DetectEngine(); err == nil {
		if got, want := EngineBinary(gotEngine), EngineBinary(detected); got != want {
			t.Errorf("the probed engine resolves to %q, want the installed engine's %q", got, want)
		}
	}
}
