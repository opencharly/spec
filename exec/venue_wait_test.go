package exec

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestReadinessEngine pins the probe-BINARY resolution: the host's configured
// engine WORD (CHARLY_RUN_ENGINE) when set, else the default. This is the branch
// WaitForContainerReady uses, so a containerd host probes with nerdctl, never
// `containerd exec`.
func TestReadinessEngine(t *testing.T) {
	cases := map[string]string{
		"podman":     "podman",
		"docker":     "docker",
		"containerd": "nerdctl", // the engine word is containerd; the probe needs its CLI
		"":           spec.EngineBinaries[defaultContainerEngine],
		// An unknown word resolves to the default engine's binary, matching
		// NestedJump.engineBinary().
		"bogus": spec.EngineBinaries[defaultContainerEngine],
	}
	for in, want := range cases {
		if got := readinessEngine(in); got != want {
			t.Errorf("readinessEngine(%q) = %q, want %q", in, got, want)
		}
	}
	// No engine word may resolve to itself when the binary differs: the
	// containerd arm above is the regression gate for the `containerd exec` bug.
	if got := readinessEngine("containerd"); got == "containerd" {
		t.Fatal("readinessEngine(containerd) returned the daemon word; the probe would run `containerd exec`")
	}
	for _, name := range spec.EngineNames {
		if got, want := readinessEngine(name), spec.EngineBinaries[name]; got != want {
			t.Errorf("readinessEngine(%q) = %q, want spec.EngineBinaries %q", name, got, want)
		}
	}
}
