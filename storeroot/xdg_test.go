package storeroot

import (
	"path/filepath"
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
		Data:       filepath.Join(home, "xdg-data"),
		Config:     filepath.Join(home, "xdg-config"),
		Cache:      filepath.Join(home, "xdg-cache"),
		State:      filepath.Join(home, "xdg-state"),
		Runtime:    filepath.Join(home, "xdg-run"),
		RuntimeSet: true,
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
		Data:       filepath.Join(home, ".local", "share"),
		Config:     filepath.Join(home, ".config"),
		Cache:      filepath.Join(home, ".cache"),
		State:      filepath.Join(home, ".local", "state"),
		Runtime:    filepath.Join(home, ".local", "state"),
		RuntimeSet: false,
	}
	if bases != want {
		t.Errorf("XDG() = %+v, want %+v", bases, want)
	}
}

// clearXDG pins every XDG variable and the cache override to the empty string, so a
// test that exercises one value never observes the developer host's real directories.
func clearXDG(t *testing.T) {
	t.Helper()
	for _, v := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(v, "")
	}
	t.Setenv("CHARLY_CACHE_DIR", "")
}

// TestXDGRelativeValueIsIgnored pins the specification rule: a value that is not an
// absolute path is invalid and MUST be ignored.
func TestXDGRelativeValueIsIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearXDG(t)
	t.Setenv("XDG_DATA_HOME", "relative/share")

	bases, err := XDG()
	if err != nil {
		t.Fatalf("XDG: %v", err)
	}
	if want := filepath.Join(home, ".local", "share"); bases.Data != want {
		t.Errorf("a relative XDG_DATA_HOME must be ignored: got %q, want %q", bases.Data, want)
	}
	if bases.RuntimeSet {
		t.Error("a relative XDG_RUNTIME_DIR must not be reported as set")
	}
}

// TestDirAndCacheDirFollowXDG pins the REWIRED callers: a relative XDG value is ignored
// by Dir and CacheDir exactly as it is by XDG(). The previous implementations
// (os.UserConfigDir, os.UserCacheDir and a raw XDG_STATE_HOME read) returned the
// relative value, so this test fails without the change.
func TestDirAndCacheDirFollowXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("XDG_STATE_HOME", "relative/state")
	t.Setenv("XDG_CACHE_HOME", "relative/cache")
	t.Setenv("CHARLY_CACHE_DIR", "")

	for root, want := range map[Root]string{
		RootConfig: filepath.Join(home, ".config", "charly"),
		RootState:  filepath.Join(home, ".local", "state", "charly"),
		RootCache:  filepath.Join(home, ".cache", "charly"),
	} {
		got, err := Dir(root)
		if err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		}
		if got != want {
			t.Errorf("Dir(%s) with a relative XDG value = %q, want %q", root, got, want)
		}
	}
}

// TestDirRuntimeFallback pins the runtime decision: a valid absolute XDG_RUNTIME_DIR
// selects the runtime root even without $HOME, while an unset or relative value falls
// back to the documented state root (D-STORE-1).
func TestDirRuntimeFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearXDG(t)

	t.Setenv("XDG_RUNTIME_DIR", "relative/run")
	if got, want := mustDir(t, RootRuntime), filepath.Join(home, ".local", "state", "charly"); got != want {
		t.Errorf("relative XDG_RUNTIME_DIR: Dir(RootRuntime) = %q, want the state fallback %q", got, want)
	}

	// A valid runtime dir answers even when $HOME is unavailable: XDG() reports the
	// missing home, but the runtime root does not depend on it.
	t.Setenv("HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got, want := mustDir(t, RootRuntime), "/run/user/1000/charly"; got != want {
		t.Errorf("absolute XDG_RUNTIME_DIR without $HOME: Dir(RootRuntime) = %q, want %q", got, want)
	}
}

func mustDir(t *testing.T, r Root) string {
	t.Helper()
	got, err := Dir(r)
	if err != nil {
		t.Fatalf("Dir(%s): %v", r, err)
	}
	return got
}

// TestDirWithAbsoluteVariablesNeedsNoHome pins that an absolute variable answers even when
// $HOME is unavailable: the old resolvers returned the variable directly, so failing on
// the unrelated missing home would be a regression. `XDG()` reports the missing home, but
// each root only needs its OWN base.
func TestDirWithAbsoluteVariablesNeedsNoHome(t *testing.T) {
	clearXDG(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/abs/config")
	t.Setenv("XDG_STATE_HOME", "/abs/state")
	t.Setenv("XDG_CACHE_HOME", "/abs/cache")
	t.Setenv("XDG_RUNTIME_DIR", "/abs/run")

	for root, want := range map[Root]string{
		RootConfig:  "/abs/config/charly",
		RootState:   "/abs/state/charly",
		RootCache:   "/abs/cache/charly",
		RootRuntime: "/abs/run/charly",
	} {
		if got := mustDir(t, root); got != want {
			t.Errorf("Dir(%s) without $HOME = %q, want %q", root, got, want)
		}
	}
	cache, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir without $HOME: %v", err)
	}
	if cache != "/abs/cache/charly" {
		t.Errorf("CacheDir() without $HOME = %q, want /abs/cache/charly", cache)
	}
}

// TestXDGAbsoluteVariablesNeedNoHome pins the error contract: when every variable is
// absolute, the bases do not depend on $HOME and XDG() must not error.
func TestXDGAbsoluteVariablesNeedNoHome(t *testing.T) {
	clearXDG(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "/abs/data")
	t.Setenv("XDG_CONFIG_HOME", "/abs/config")
	t.Setenv("XDG_CACHE_HOME", "/abs/cache")
	t.Setenv("XDG_STATE_HOME", "/abs/state")
	t.Setenv("XDG_RUNTIME_DIR", "/abs/run")

	bases, err := XDG()
	if err != nil {
		t.Fatalf("XDG with every variable absolute and no $HOME: %v", err)
	}
	if bases.Data != "/abs/data" || bases.Config != "/abs/config" || bases.Cache != "/abs/cache" || bases.State != "/abs/state" || bases.Runtime != "/abs/run" {
		t.Errorf("XDG() = %+v", bases)
	}
	if !bases.RuntimeSet {
		t.Error("an absolute XDG_RUNTIME_DIR must be reported as set")
	}
}

// TestXDGUnresolvedBaseErrorsWithoutHome pins the other half of the contract: a base that
// had to fall back to a relative default because $HOME is unavailable yields the error,
// while an absolute base in the same call still resolves.
func TestXDGUnresolvedBaseErrorsWithoutHome(t *testing.T) {
	clearXDG(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "/abs/data")

	bases, err := XDG()
	if err == nil {
		t.Fatal("XDG must error when unset bases fell back to relative defaults without $HOME")
	}
	if bases.Data != "/abs/data" {
		t.Errorf("bases.Data = %q, want the absolute data base", bases.Data)
	}
}
