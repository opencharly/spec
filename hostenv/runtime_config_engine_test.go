package hostenv

import "testing"

func TestValidateEngine_AcceptsNerdctl(t *testing.T) {
	for _, engine := range []string{"podman", "docker", "nerdctl"} {
		if err := ValidateEngine(engine, "engine.run"); err != nil {
			t.Errorf("ValidateEngine(%q) unexpectedly rejected: %v", engine, err)
		}
	}
	// containerd is still not an engine word — nerdctl is the containerd CLI,
	// containerd itself is a daemon, not a CLI engine.
	if err := ValidateEngine("containerd", "engine.run"); err == nil {
		t.Error("ValidateEngine(containerd) must still be rejected")
	}
	if err := ValidateEngine("", "engine.run"); err == nil {
		t.Error("ValidateEngine(\"\") must be rejected")
	}
}

func TestValidateRunMode_AcceptsSystemdUnit(t *testing.T) {
	for _, mode := range []string{"auto", "direct", "quadlet", "systemd-unit"} {
		if err := ValidateRunMode(mode); err != nil {
			t.Errorf("ValidateRunMode(%q) unexpectedly rejected: %v", mode, err)
		}
	}
	if err := ValidateRunMode("bogus"); err == nil {
		t.Error("ValidateRunMode(bogus) must be rejected")
	}
}
