package storeroot

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestXDGResolvesEveryBase pins the five bases from an absolute value for each, so a
// base that is dropped or crossed with another is a failure and not a new truth.
func TestXDGResolvesEveryBase(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))

	bases, err := XDG()
	if err != nil {
		t.Fatalf("XDG: %v", err)
	}
	want := Bases{
		Data:    filepath.Join(home, "xdg-data"),
		Config:  filepath.Join(home, "xdg-config"),
		Cache:   filepath.Join(home, "xdg-cache"),
		State:   filepath.Join(home, "xdg-state"),
		Runtime: filepath.Join(home, "xdg-run"),
	}
	if bases != want {
		t.Errorf("XDG() = %+v, want %+v", bases, want)
	}
}

// TestXDGDefaultsWithoutVariables pins the specification's Unix defaults.
func TestXDGDefaultsWithoutVariables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(v, "")
	}

	bases, err := XDG()
	if err != nil {
		t.Fatalf("XDG: %v", err)
	}
	want := Bases{
		Data:    filepath.Join(home, ".local", "share"),
		Config:  filepath.Join(home, ".config"),
		Cache:   filepath.Join(home, ".cache"),
		State:   filepath.Join(home, ".local", "state"),
		Runtime: filepath.Join("/run/user", strconv.Itoa(os.Getuid())),
	}
	if bases != want {
		t.Errorf("XDG() = %+v, want %+v", bases, want)
	}
}

// TestXDGRelativeValueIsIgnored pins the specification rule: a value that is not an
// absolute path is invalid and MUST be ignored.
func TestXDGRelativeValueIsIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative/share")

	bases, err := XDG()
	if err != nil {
		t.Fatalf("XDG: %v", err)
	}
	if want := filepath.Join(home, ".local", "share"); bases.Data != want {
		t.Errorf("a relative XDG_DATA_HOME must be ignored: got %q, want %q", bases.Data, want)
	}
}
