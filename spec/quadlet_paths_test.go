package spec

import (
	"path/filepath"
	"testing"
)

// TestQuadletDirsFollowXDGConfigHome is the R7 control for the quadlet/systemd
// migration: the pre-change helpers called os.UserHomeDir() and ignored
// XDG_CONFIG_HOME, so both assertions fail on that tree.
func TestQuadletDirsFollowXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(t.TempDir(), "xdg-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)

	qdir, err := QuadletDir()
	if err != nil {
		t.Fatalf("QuadletDir: %v", err)
	}
	if want := filepath.Join(config, "containers", "systemd"); qdir != want {
		t.Errorf("QuadletDir() = %q, want %q", qdir, want)
	}
	sdir, err := SystemdUserDir()
	if err != nil {
		t.Fatalf("SystemdUserDir: %v", err)
	}
	if want := filepath.Join(config, "systemd", "user"); sdir != want {
		t.Errorf("SystemdUserDir() = %q, want %q", sdir, want)
	}
}

// TestQuadletDirsFallBackToHome pins the XDG default when XDG_CONFIG_HOME is unset.
func TestQuadletDirsFallBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	qdir, err := QuadletDir()
	if err != nil {
		t.Fatalf("QuadletDir: %v", err)
	}
	if want := filepath.Join(home, ".config", "containers", "systemd"); qdir != want {
		t.Errorf("QuadletDir() = %q, want %q", qdir, want)
	}
	sdir, err := SystemdUserDir()
	if err != nil {
		t.Fatalf("SystemdUserDir: %v", err)
	}
	if want := filepath.Join(home, ".config", "systemd", "user"); sdir != want {
		t.Errorf("SystemdUserDir() = %q, want %q", sdir, want)
	}
}
