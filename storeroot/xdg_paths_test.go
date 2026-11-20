package storeroot

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestBaseAndDirUseTheRightSuffix pins the split between an XDG base (another app's
// directory) and charly's own root (base/charly). The cache root is the one special
// case: CHARLY_CACHE_DIR is a complete root and must never gain a charly suffix.
func TestBaseAndDirUseTheRightSuffix(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	for root, base := range map[Root]string{
		RootConfig:  filepath.Join(home, "xdg-config"),
		RootData:    filepath.Join(home, "xdg-data"),
		RootCache:   filepath.Join(home, "xdg-cache"),
		RootState:   filepath.Join(home, "xdg-state"),
		RootRuntime: filepath.Join(home, "xdg-run"),
	} {
		gotBase, err := Base(root)
		if err != nil {
			t.Fatalf("Base(%s): %v", root, err)
		}
		if gotBase != base {
			t.Errorf("Base(%s) = %q, want the unsuffixed base %q", root, gotBase, base)
		}
		gotDir, err := Dir(root)
		if err != nil {
			t.Fatalf("Dir(%s): %v", root, err)
		}
		if want := filepath.Join(base, "charly"); gotDir != want {
			t.Errorf("Dir(%s) = %q, want %q", root, gotDir, want)
		}
	}

	if got, err := DataDir(); err != nil || got != filepath.Join(home, "xdg-data", "charly") {
		t.Errorf("DataDir() = %q, %v; want the data root", got, err)
	}

	override := filepath.Join(home, "relocated-cache")
	t.Setenv("CHARLY_CACHE_DIR", override)
	if got, err := Base(RootCache); err != nil || got != override {
		t.Errorf("Base(RootCache) with the override = %q, %v; want the override verbatim", got, err)
	}
	if got, err := Dir(RootCache); err != nil || got != override {
		t.Errorf("Dir(RootCache) with the override = %q, %v; want the override verbatim", got, err)
	}
}

// TestBaseDirsDefaultsAndOrder pins the specification's search-path rules: the default
// when the variable is unset or empty, the declared order when it is set, and the
// discard of a non-absolute entry.
func TestBaseDirsDefaultsAndOrder(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	t.Setenv("XDG_CONFIG_DIRS", "/etc/a:/etc/b")
	if got, want := mustBaseDirs(t, RootConfig), []string{"/etc/a", "/etc/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseDirs(RootConfig) = %v, want %v", got, want)
	}
	if got, want := mustDirs(t, RootConfig), []string{filepath.Join(home, "xdg-config", "charly"), "/etc/a/charly", "/etc/b/charly"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Dirs(RootConfig) = %v, want %v", got, want)
	}

	t.Setenv("XDG_DATA_DIRS", "/d1::relative:/d2")
	if got, want := mustBaseDirs(t, RootData), []string{"/d1", "/d2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseDirs(RootData) = %v, want %v", got, want)
	}

	t.Setenv("XDG_CONFIG_DIRS", "")
	if got, want := mustBaseDirs(t, RootConfig), []string{"/etc/xdg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseDirs(RootConfig) with an empty variable = %v, want the default %v", got, want)
	}
	t.Setenv("XDG_DATA_DIRS", "")
	if got, want := mustBaseDirs(t, RootData), []string{"/usr/local/share", "/usr/share"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseDirs(RootData) with an empty variable = %v, want the default %v", got, want)
	}

	if _, err := BaseDirs(RootCache); err == nil {
		t.Error("BaseDirs(RootCache) = nil error; the cache root has no search path")
	}
}

// TestSearchFindsTheFirstExistingMatch pins the precedence: charly's own tree first, then
// the search entries in order; a missing file is not an error; an absolute or traversing
// relative path is.
func TestSearchFindsTheFirstExistingMatch(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)
	first, second := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_DIRS", first+":"+second)

	writeFile(t, filepath.Join(second, "charly", "thing"), "second")
	if got, found, err := Search(RootData, "thing"); err != nil || !found || got != filepath.Join(second, "charly", "thing") {
		t.Fatalf("Search(RootData, thing) = %q, %v, %v; want the second search entry", got, found, err)
	}

	writeFile(t, filepath.Join(home, "xdg-data", "charly", "thing"), "home")
	if got, found, err := Search(RootData, "thing"); err != nil || !found || got != filepath.Join(home, "xdg-data", "charly", "thing") {
		t.Errorf("Search(RootData, thing) = %q, %v, %v; want the home data root first", got, found, err)
	}

	if got, found, err := Search(RootData, "missing"); err != nil || found || got != "" {
		t.Errorf("Search(RootData, missing) = %q, %v, %v; want (\"\", false, nil)", got, found, err)
	}
	if _, _, err := Search(RootData, "/abs"); err == nil {
		t.Error("Search accepted an absolute path")
	}
	if _, _, err := Search(RootData, "../escape"); err == nil {
		t.Error("Search accepted a .. element")
	}
}

// TestResolverUsesInjectedBases proves the D-STORE-8 seam: the resolver answers from the
// injected bases and environment, never from the process environment.
func TestResolverUsesInjectedBases(t *testing.T) {
	r := &Resolver{
		Bases: func() (Bases, error) {
			return Bases{
				Data:       "/x/data",
				Config:     "/x/config",
				Cache:      "/x/cache",
				State:      "/x/state",
				Runtime:    "/x/run",
				RuntimeSet: true,
			}, nil
		},
		LookupEnv: func(key string) (string, bool) {
			if key == "XDG_DATA_DIRS" {
				return "/x/share", true
			}
			return "", false
		},
	}

	if got, err := r.Dir(RootData); err != nil || got != "/x/data/charly" {
		t.Errorf("injected Dir(RootData) = %q, %v", got, err)
	}
	if got, err := r.Base(RootConfig); err != nil || got != "/x/config" {
		t.Errorf("injected Base(RootConfig) = %q, %v", got, err)
	}
	if got, err := r.BaseDirs(RootData); err != nil || !reflect.DeepEqual(got, []string{"/x/share"}) {
		t.Errorf("injected BaseDirs(RootData) = %v, %v", got, err)
	}
	if got, err := r.Dirs(RootData); err != nil || !reflect.DeepEqual(got, []string{"/x/data/charly", "/x/share/charly"}) {
		t.Errorf("injected Dirs(RootData) = %v, %v", got, err)
	}
}

// TestDataRootIsNotTheCacheRoot pins the semantic split: relocating one root must not
// move the other, and the data root must not be a second cache.
func TestDataRootIsNotTheCacheRoot(t *testing.T) {
	home := t.TempDir()
	pinXDG(t, home)

	data, err := Dir(RootData)
	if err != nil {
		t.Fatalf("Dir(RootData): %v", err)
	}
	cache, err := Dir(RootCache)
	if err != nil {
		t.Fatalf("Dir(RootCache): %v", err)
	}
	if data == cache {
		t.Fatalf("Dir(RootData) == Dir(RootCache) = %q; data and cache are different trees", data)
	}
}

func mustBaseDirs(t *testing.T, root Root) []string {
	t.Helper()
	got, err := BaseDirs(root)
	if err != nil {
		t.Fatalf("BaseDirs(%s): %v", root, err)
	}
	return got
}

func mustDirs(t *testing.T, root Root) []string {
	t.Helper()
	got, err := Dirs(root)
	if err != nil {
		t.Fatalf("Dirs(%s): %v", root, err)
	}
	return got
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
