package hostenv

import (
	"path/filepath"
	"testing"
)

// setXDG points every XDG base directory variable at an absolute test path.
func setXDG(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	t.Setenv("XDG_RUNTIME_DIR", "/xdg/run")
}

func TestExpandHostHomeXDGVars(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	setXDG(t)

	tests := map[string]string{
		"$XDG_DATA_HOME/charly/models":    "/xdg/data/charly/models",
		"${XDG_DATA_HOME}/charly/volumes": "/xdg/data/charly/volumes",
		"$XDG_CONFIG_HOME/charly":         "/xdg/config/charly",
		"$XDG_CACHE_HOME/charly":          "/xdg/cache/charly",
		"$XDG_STATE_HOME/charly":          "/xdg/state/charly",
		"$XDG_RUNTIME_DIR/charly":         "/xdg/run/charly",
		"~/models":                        filepath.Join(home, "models"),
		"~":                               home,
		"$HOME/models":                    filepath.Join(home, "models"),
		"${HOME}/models":                  filepath.Join(home, "models"),
		"/absolute/untouched":             "/absolute/untouched",
	}
	for in, want := range tests {
		if got := ExpandHostHome(in); got != want {
			t.Errorf("ExpandHostHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandHostHomeXDGDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// An empty variable and an unset variable are equivalent to the specification.
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	tests := map[string]string{
		"$XDG_DATA_HOME/x":   filepath.Join(home, ".local", "share", "x"),
		"$XDG_CONFIG_HOME/x": filepath.Join(home, ".config", "x"),
		"$XDG_CACHE_HOME/x":  filepath.Join(home, ".cache", "x"),
		"$XDG_STATE_HOME/x":  filepath.Join(home, ".local", "state", "x"),
	}
	for in, want := range tests {
		if got := ExpandHostHome(in); got != want {
			t.Errorf("ExpandHostHome(%q) = %q, want %q", in, got, want)
		}
	}
	// The runtime base follows D-STORE-1: with XDG_RUNTIME_DIR unset it is the state base,
	// never /run/user/<uid> — the same value Dir(RootRuntime) uses.
	wantRuntime := filepath.Join(home, ".local", "state", "x")
	if got := ExpandHostHome("$XDG_RUNTIME_DIR/x"); got != wantRuntime {
		t.Errorf("runtime default = %q, want %q", got, wantRuntime)
	}
}

// TestExpandHostHomeUnresolvedTokenStaysLiteral pins that an unresolved base (the variable
// unset or relative and $HOME unavailable) is left as a literal token rather than replaced
// with a relative path that would become a container bind.
func TestExpandHostHomeUnresolvedTokenStaysLiteral(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	if got, want := ExpandHostHome("$XDG_DATA_HOME/x"), "$XDG_DATA_HOME/x"; got != want {
		t.Errorf("unresolved XDG_DATA_HOME = %q, want the literal token %q", got, want)
	}
	if got, want := ExpandHostHome("$XDG_CONFIG_HOME/x"), "$XDG_CONFIG_HOME/x"; got != want {
		t.Errorf("relative XDG_CONFIG_HOME = %q, want the literal token %q", got, want)
	}
	if got, want := ExpandHostHome("$XDG_RUNTIME_DIR/x"), "$XDG_RUNTIME_DIR/x"; got != want {
		t.Errorf("unresolved XDG_RUNTIME_DIR = %q, want the literal token %q", got, want)
	}
}

func TestExpandHostHomeXDGRelativeIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative/share")

	if got, want := ExpandHostHome("$XDG_DATA_HOME/x"), filepath.Join(home, ".local", "share", "x"); got != want {
		t.Errorf("a relative XDG_DATA_HOME must be ignored: got %q, want %q", got, want)
	}
	if got, want := ResolveVolumesPath("", ""), filepath.Join(home, ".local", "share", "charly", "volumes"); got != want {
		t.Errorf("ResolveVolumesPath default with a relative XDG_DATA_HOME = %q, want %q", got, want)
	}
}

func TestResolveVolumesPathXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	if got := ResolveVolumesPath("", ""); got != "/xdg/data/charly/volumes" {
		t.Errorf("default = %q, want /xdg/data/charly/volumes", got)
	}
	if got := ResolveVolumesPath("/env/volumes", ""); got != "/env/volumes" {
		t.Errorf("env precedence = %q", got)
	}
	if got := ResolveVolumesPath("", "~/cfg/volumes"); got != filepath.Join(home, "cfg", "volumes") {
		t.Errorf("config expansion = %q", got)
	}
}

func TestResolveEncryptedStoragePathXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	if got := ResolveEncryptedStoragePath("", ""); got != "/xdg/data/charly/encrypted" {
		t.Errorf("default = %q, want /xdg/data/charly/encrypted", got)
	}
	if got := ResolveEncryptedStoragePath("/env/enc", ""); got != "/env/enc" {
		t.Errorf("env precedence = %q", got)
	}
}

// TestResolvePathsAbsoluteDataNeedsNoHome pins that the data base answers from an
// absolute XDG_DATA_HOME without $HOME: the old resolvers never needed the home for an
// absolute variable, so falling back to a relative default here would be a regression.
func TestResolvePathsAbsoluteDataNeedsNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "/abs/data")

	if got, want := ResolveVolumesPath("", ""), "/abs/data/charly/volumes"; got != want {
		t.Errorf("ResolveVolumesPath without $HOME = %q, want %q", got, want)
	}
	if got, want := ResolveEncryptedStoragePath("", ""), "/abs/data/charly/encrypted"; got != want {
		t.Errorf("ResolveEncryptedStoragePath without $HOME = %q, want %q", got, want)
	}
	if got, want := ExpandHostHome("$XDG_DATA_HOME/charly/models"), "/abs/data/charly/models"; got != want {
		t.Errorf("ExpandHostHome without $HOME = %q, want %q", got, want)
	}
}
