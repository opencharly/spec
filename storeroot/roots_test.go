package storeroot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pinXDG pins every input the resolver reads, so a test never observes the developer
// host's real directories. home is used as $HOME; every XDG variable is set to a
// distinct subdirectory so a wrong root cannot coincidentally match a right one.
func pinXDG(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, "xdg-run"))
	t.Setenv("CHARLY_CACHE_DIR", "")
}

// TestDirResolvesTheFourRoots pins all four roots in the DEFAULT condition (every XDG
// variable set, the one override unset). Each expectation is spelled out rather than
// derived from the implementation, so a root that moves is a failure, not a new truth.
func TestDirResolvesTheFourRoots(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	want := map[Root]string{
		RootConfig:  filepath.Join(home, "xdg-config", "charly"),
		RootCache:   filepath.Join(home, "xdg-cache", "charly"),
		RootState:   filepath.Join(home, "xdg-state", "charly"),
		RootRuntime: filepath.Join(home, "xdg-run", "charly"),
	}
	for root, wantDir := range want {
		got, err := Dir(root)
		if err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		}
		if got != wantDir {
			t.Errorf("Dir(%s) = %q, want %q", root, got, wantDir)
		}
	}

	cache, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if cache != want[RootCache] {
		t.Errorf("CacheDir() = %q, want the cache root %q", cache, want[RootCache])
	}
}

// TestCacheOverrideRelocatesOnlyTheCacheRoot pins the ONE override: CHARLY_CACHE_DIR
// moves the cache root, and no other root follows it. An override that also moved the
// state or runtime root would relocate locks and per-boot files into a cache the
// operator expects to be disposable.
func TestCacheOverrideRelocatesOnlyTheCacheRoot(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	relocated := filepath.Join(home, "relocated-store")
	t.Setenv("CHARLY_CACHE_DIR", relocated)

	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if got != relocated {
		t.Fatalf("CacheDir() = %q, want the CHARLY_CACHE_DIR override %q", got, relocated)
	}
	viaDir, err := Dir(RootCache)
	if err != nil {
		t.Fatalf("Dir(RootCache): %v", err)
	}
	if viaDir != relocated {
		t.Errorf("Dir(RootCache) = %q, want the override %q — Dir and CacheDir must not disagree about the cache root", viaDir, relocated)
	}

	for root, wantDir := range map[Root]string{
		RootConfig:  filepath.Join(home, "xdg-config", "charly"),
		RootState:   filepath.Join(home, "xdg-state", "charly"),
		RootRuntime: filepath.Join(home, "xdg-run", "charly"),
	} {
		if gotDir, err := Dir(root); err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		} else if gotDir != wantDir {
			t.Errorf("Dir(%s) = %q with CHARLY_CACHE_DIR set, want %q — the override relocates the cache root ONLY", root, gotDir, wantDir)
		}
	}
}

// TestCacheOverrideIsUsedVerbatim pins the resolver's contract with the operator's
// value: it is never absolutised, cleaned or otherwise rewritten, so a relative value
// stays relative. Pinned because "helpfully" absolutising it would silently change
// which directory every store lands in for a caller whose working directory differs.
func TestCacheOverrideIsUsedVerbatim(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	t.Setenv("CHARLY_CACHE_DIR", "relative-cache")

	for _, name := range []string{"CacheDir", "Dir(RootCache)"} {
		var got string
		var err error
		if name == "CacheDir" {
			got, err = CacheDir()
		} else {
			got, err = Dir(RootCache)
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != "relative-cache" {
			t.Errorf("%s = %q, want the operator's value verbatim (a relative override stays relative)", name, got)
		}
	}
}

// TestRootsFallBackToHomeWithoutXDG pins the no-XDG condition: the config/cache roots
// are the XDG defaults under $HOME, the state root is `$HOME/.local/state/charly`, and
// the runtime root — having no per-boot directory to use — is the STATE root rather
// than /tmp, which is shared and world-writable.
func TestRootsFallBackToHomeWithoutXDG(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(v, "")
	}

	want := map[Root]string{
		RootConfig:  filepath.Join(home, ".config", "charly"),
		RootCache:   filepath.Join(home, ".cache", "charly"),
		RootState:   filepath.Join(home, ".local", "state", "charly"),
		RootRuntime: filepath.Join(home, ".local", "state", "charly"),
	}
	for root, wantDir := range want {
		got, err := Dir(root)
		if err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		}
		if got != wantDir {
			t.Errorf("Dir(%s) = %q, want %q", root, got, wantDir)
		}
	}
	// The fallback must not be the SHARED temp directory. (A test's own HOME often IS
	// under /tmp, so a prefix test would confuse "the operator's home happens to be
	// there" with "the resolver picked the shared directory" — compare the directory
	// itself, and its charly child, instead.)
	rt, err := Dir(RootRuntime)
	if err != nil {
		t.Fatalf("Dir(RootRuntime): %v", err)
	}
	for _, shared := range []string{os.TempDir(), filepath.Join(os.TempDir(), "charly")} {
		if rt == shared {
			t.Errorf("Dir(RootRuntime) = %q — the no-runtime-dir fallback must be the state root, never the shared temp directory", rt)
		}
	}
}

// TestRuntimeRootFallsBackToStateAlone isolates the fallback from the other three
// roots: only XDG_RUNTIME_DIR is absent, so nothing but the runtime root may move.
func TestRuntimeRootFallsBackToStateAlone(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	t.Setenv("XDG_RUNTIME_DIR", "")

	state, err := Dir(RootState)
	if err != nil {
		t.Fatalf("Dir(RootState): %v", err)
	}
	runtime, err := Dir(RootRuntime)
	if err != nil {
		t.Fatalf("Dir(RootRuntime): %v", err)
	}
	if runtime != state {
		t.Errorf("Dir(RootRuntime) = %q with XDG_RUNTIME_DIR unset, want the state root %q", runtime, state)
	}
	if state != filepath.Join(home, "xdg-state", "charly") {
		t.Errorf("Dir(RootState) = %q, want %q — the runtime fallback must not move the state root it falls back to", state, filepath.Join(home, "xdg-state", "charly"))
	}
}

// TestUnknownRootIsAnError pins the closed root set: a misspelt root is an error naming
// the four valid ones, never a silently-created directory named after the typo.
func TestUnknownRootIsAnError(t *testing.T) {
	pinXDG(t, t.TempDir())

	got, err := Dir(Root("cach"))
	if err == nil {
		t.Fatalf("Dir(%q) = %q, want an error — the root set is closed", "cach", got)
	}
	for _, valid := range []string{"config", "cache", "state", "runtime"} {
		if !strings.Contains(err.Error(), valid) {
			t.Errorf("error %q does not name the valid root %q — a caller cannot correct the typo from it", err, valid)
		}
	}
}
