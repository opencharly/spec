package hostenv

import (
	"os/exec"
	"testing"
)

// runModeCase pins the engine→mode resolution on a host WITH a systemd-user
// session (the seam dir is pointed at a real temp dir) so the "direct" fallback
// is not what the assertion is accidentally measuring.
func withSystemdUserSession(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("systemctl not present; DetectRunMode cannot reach a unit mode")
	}
	orig := SystemdUserRuntimeDir
	SystemdUserRuntimeDir = func() string { return t.TempDir() }
	t.Cleanup(func() { SystemdUserRuntimeDir = orig })
}

// TestDetectRunMode_NerdctlIsSystemdUnit is the new-behaviour gate: with a
// systemd-user session, nerdctl resolves to the systemd-unit mode (it has no
// quadlet generator), not "direct" and not "quadlet".
func TestDetectRunMode_NerdctlIsSystemdUnit(t *testing.T) {
	withSystemdUserSession(t)
	if got := DetectRunMode("nerdctl"); got != "systemd-unit" {
		t.Errorf("DetectRunMode(nerdctl) = %q, want systemd-unit", got)
	}
}

func TestDetectRunMode_PodmanIsQuadlet(t *testing.T) {
	withSystemdUserSession(t)
	if got := DetectRunMode("podman"); got != "quadlet" {
		t.Errorf("DetectRunMode(podman) = %q, want quadlet", got)
	}
	// Regression: docker has no unit mode.
	if got := DetectRunMode("docker"); got != "direct" {
		t.Errorf("DetectRunMode(docker) = %q, want direct", got)
	}
}

// TestDetectRunMode_NoSystemdUserSessionFallsBackToDirect proves the
// host-degraded path: a unit-capable engine still resolves to "direct" when no
// functional systemd-user session exists. The seam dir is pointed at a
// non-existent path so the probe fails deterministically.
func TestDetectRunMode_NoSystemdUserSessionFallsBackToDirect(t *testing.T) {
	orig := SystemdUserRuntimeDir
	SystemdUserRuntimeDir = func() string { return "/nonexistent/charly-test-no-systemd-user" }
	t.Cleanup(func() { SystemdUserRuntimeDir = orig })
	if got := DetectRunMode("nerdctl"); got != "direct" {
		t.Errorf("DetectRunMode(nerdctl) with no systemd-user session = %q, want direct", got)
	}
	if got := DetectRunMode("podman"); got != "direct" {
		t.Errorf("DetectRunMode(podman) with no systemd-user session = %q, want direct", got)
	}
}

// TestRunModeMismatchWarning is the coverage gate for the ResolveRuntime
// warning branch: a unit run_mode the resolved engine cannot offer warns; a
// matching pair does not; and `direct` never warns (it is the host-degraded
// fallback, not a mismatch).
func TestRunModeMismatchWarning(t *testing.T) {
	cases := []struct {
		engine, mode string
		wantWarn     bool
	}{
		{"podman", "quadlet", false},       // match
		{"nerdctl", "systemd-unit", false}, // match
		{"nerdctl", "quadlet", true},       // nerdctl has no quadlet generator
		{"podman", "systemd-unit", true},   // podman offers quadlet, not systemd-unit
		{"docker", "systemd-unit", true},   // docker offers no unit mode
		{"nerdctl", "direct", false},       // direct is the degraded fallback
		{"podman", "direct", false},        // direct is the degraded fallback
		{"bogus", "quadlet", false},        // unknown word == the default engine (podman → quadlet)
		// The EMPTY engine means "unspecified" and resolves to the ONE default
		// engine (podman → quadlet), so an empty engine with quadlet MATCHES (no
		// warning) and an empty engine with systemd-unit is a real mismatch. This
		// is the block-1 single-home resolution reaching the warning branch.
		{"", "quadlet", false},       // empty == the default engine's mode
		{"", "systemd-unit", true},   // empty defaults to podman, which offers quadlet
	}
	for _, c := range cases {
		got := runModeMismatchWarning(c.engine, c.mode)
		if (got != "") != c.wantWarn {
			t.Errorf("runModeMismatchWarning(%q, %q) = %q, wantWarn=%v", c.engine, c.mode, got, c.wantWarn)
		}
	}
}
