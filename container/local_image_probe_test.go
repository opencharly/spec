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
		// unknown/empty falls back to the inspect form (the historical default).
		"":      {"image", "inspect"},
		"bogus": {"image", "inspect"},
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
