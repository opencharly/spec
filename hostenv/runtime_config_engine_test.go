package hostenv

import "testing"

func TestValidateEngine_AcceptsContainerd(t *testing.T) {
	for _, engine := range []string{"podman", "docker", "containerd"} {
		if err := ValidateEngine(engine, "engine.run"); err != nil {
			t.Errorf("ValidateEngine(%q) unexpectedly rejected: %v", engine, err)
		}
	}
	// nerdctl is the containerd CLI, never an engine word: the deleted spelling
	// must be rejected by the same vocabulary gate.
	if err := ValidateEngine("nerdctl", "engine.run"); err == nil {
		t.Error("ValidateEngine(nerdctl) must be rejected — the engine word is containerd")
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
