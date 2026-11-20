package storeroot

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// testOverrides spells out the charly-only override variable per root, independently of
// the resolver, so this test is a statement of the contract rather than a mirror of the
// implementation.
var testOverrides = map[Root]string{
	RootConfig:  "CHARLY_CONFIG_HOME",
	RootData:    "CHARLY_DATA_HOME",
	RootCache:   "CHARLY_CACHE_HOME",
	RootState:   "CHARLY_STATE_HOME",
	RootRuntime: "CHARLY_RUNTIME_DIR",
}

// TestCharlyOverridesRelocateEveryRoot pins the charly-only override contract (D-STORE-1):
// each XDG input has one override that names a COMPLETE charly root, so it is returned
// verbatim rather than having `charly` joined onto it. The expectations are spelled out,
// so a root that ignores its override is a failure.
func TestCharlyOverridesRelocateEveryRoot(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	overrides := make(map[Root]string, len(testOverrides))
	for root, env := range testOverrides {
		dir := filepath.Join(home, "charly-"+string(root))
		t.Setenv(env, dir)
		overrides[root] = dir
	}

	for root, want := range overrides {
		got, err := Dir(root)
		if err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		}
		if got != want {
			t.Errorf("Dir(%s) = %q, want the complete charly root %q", root, got, want)
		}
	}

	cache, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if cache != overrides[RootCache] {
		t.Errorf("CacheDir() = %q, want CHARLY_CACHE_HOME %q", cache, overrides[RootCache])
	}
	base, err := Base(RootCache)
	if err != nil {
		t.Fatalf("Base(RootCache): %v", err)
	}
	if base != overrides[RootCache] {
		t.Errorf("Base(RootCache) = %q, want %q — a charly-owned sibling of the cache root must land under the override", base, overrides[RootCache])
	}
}

// TestCharlyOverrideRelativeOrEmptyIsIgnored pins the invalid-value rule: an override that
// is not an absolute path is ignored, so the XDG input's default applies.
func TestCharlyOverrideRelativeOrEmptyIsIgnored(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	want := map[Root]string{
		RootConfig:  filepath.Join(home, "xdg-config", "charly"),
		RootData:    filepath.Join(home, "xdg-data", "charly"),
		RootCache:   filepath.Join(home, "xdg-cache", "charly"),
		RootState:   filepath.Join(home, "xdg-state", "charly"),
		RootRuntime: filepath.Join(home, "xdg-run", "charly"),
	}
	for root, env := range testOverrides {
		t.Setenv(env, "relative/"+string(root))
		if got, err := Dir(root); err != nil {
			t.Fatalf("Dir(%s) with a relative override: %v", root, err)
		} else if got != want[root] {
			t.Errorf("Dir(%s) = %q with a relative %s, want the XDG default %q — a relative override is invalid and ignored", root, got, env, want[root])
		}
	}
}

// TestCharlyCacheHomeWinsOverCacheDir pins the precedence D-STORE-1 states for the two
// cache overrides, and keeps `CHARLY_CACHE_DIR` — which is never dropped — in effect when
// the newer override is absent.
func TestCharlyCacheHomeWinsOverCacheDir(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	legacy := filepath.Join(home, "legacy-cache")
	both := filepath.Join(home, "new-cache")
	t.Setenv("CHARLY_CACHE_DIR", legacy)

	if got, err := CacheDir(); err != nil {
		t.Fatalf("CacheDir: %v", err)
	} else if got != legacy {
		t.Fatalf("CacheDir() = %q, want the kept CHARLY_CACHE_DIR %q", got, legacy)
	}

	t.Setenv("CHARLY_CACHE_HOME", both)
	for _, name := range []string{"CacheDir", "Dir(RootCache)", "Base(RootCache)"} {
		var got string
		var err error
		switch name {
		case "CacheDir":
			got, err = CacheDir()
		case "Dir(RootCache)":
			got, err = Dir(RootCache)
		default:
			got, err = Base(RootCache)
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != both {
			t.Errorf("%s = %q when both cache overrides are set, want CHARLY_CACHE_HOME %q", name, got, both)
		}
	}
}

// TestCharlyOverridesDoNotMoveAnXDGBase pins the boundary: an override names a COMPLETE
// charly root, so a path belonging to another application keeps resolving against the XDG
// base the operator set.
func TestCharlyOverridesDoNotMoveAnXDGBase(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	t.Setenv("CHARLY_CONFIG_HOME", filepath.Join(home, "charly-config-home"))
	t.Setenv("CHARLY_RUNTIME_DIR", filepath.Join(home, "charly-runtime-dir"))

	want := map[Root]string{
		RootConfig:  filepath.Join(home, "xdg-config"),
		RootRuntime: filepath.Join(home, "xdg-run"),
	}
	for root, wantBase := range want {
		got, err := Base(root)
		if err != nil {
			t.Fatalf("Base(%s): %v", root, err)
		}
		if got != wantBase {
			t.Errorf("Base(%s) = %q with its charly override set, want the XDG base %q — an override is a complete charly root, not a base", root, got, wantBase)
		}
	}
}

// TestCharlyDirsOverrideIsCompleteCharlyDirs pins the `_DIRS` form: a colon-separated list
// of complete charly directories, used verbatim after the home root, with the XDG search
// path untouched for other applications.
func TestCharlyDirsOverrideIsCompleteCharlyDirs(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	first := filepath.Join(home, "site-charly")
	second := filepath.Join(home, "extra-charly")
	t.Setenv("CHARLY_CONFIG_DIRS", first+":"+second)
	t.Setenv("CHARLY_DATA_DIRS", first+":"+second)

	for _, root := range []Root{RootConfig, RootData} {
		homeRoot, err := Dir(root)
		if err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		}
		got, err := Dirs(root)
		if err != nil {
			t.Fatalf("Dirs(%s): %v", root, err)
		}
		want := []string{homeRoot, first, second}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Dirs(%s) = %q, want %q — the _DIRS override names complete charly directories", root, got, want)
		}
		raw, err := BaseDirs(root)
		if err != nil {
			t.Fatalf("BaseDirs(%s): %v", root, err)
		}
		if slices.Contains(raw, first) || slices.Contains(raw, second) {
			t.Errorf("BaseDirs(%s) = %q, want the XDG input only — the _DIRS override is not an XDG base list", root, raw)
		}
	}
}

// TestCharlyDirsOverrideWithNoAbsoluteEntryIsIgnored pins the invalid-value rule for the
// `_DIRS` form, and the fallback to the XDG search path built from each base.
func TestCharlyDirsOverrideWithNoAbsoluteEntryIsIgnored(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	t.Setenv("CHARLY_CONFIG_DIRS", "relative:also-relative")

	homeRoot, err := Dir(RootConfig)
	if err != nil {
		t.Fatalf("Dir(RootConfig): %v", err)
	}
	got, err := Dirs(RootConfig)
	if err != nil {
		t.Fatalf("Dirs(RootConfig): %v", err)
	}
	want := []string{homeRoot, filepath.Join("/etc/xdg", "charly")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Dirs(RootConfig) = %q with a relative CHARLY_CONFIG_DIRS, want the XDG default path %q", got, want)
	}
}
