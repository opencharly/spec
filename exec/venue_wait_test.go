package exec

import "testing"

// TestReadinessEngine pins the probe-engine resolution: the host's configured
// engine (CHARLY_RUN_ENGINE) when set, else the default. This is the branch
// WaitForContainerReady uses, so a docker/nerdctl host probes its own binary
// instead of a hardcoded podman.
func TestReadinessEngine(t *testing.T) {
	cases := map[string]string{
		"podman":  "podman",
		"docker":  "docker",
		"nerdctl": "nerdctl",
		"":        defaultContainerEngine,
	}
	for in, want := range cases {
		if got := readinessEngine(in); got != want {
			t.Errorf("readinessEngine(%q) = %q, want %q", in, got, want)
		}
	}
}
