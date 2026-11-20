package exec

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUserScopeEnvFollowsTheCacheRoot is the R7 control for the builder-cache
// migration: PIXI_CACHE_DIR and RATTLER_CACHE_DIR now follow the one cache root,
// so CHARLY_CACHE_DIR relocates them with every store. The pre-change code pinned
// them under $HOME/.cache/charly and ignored the override.
func TestUserScopeEnvFollowsTheCacheRoot(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "relocated-cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("CHARLY_CACHE_DIR", override)

	env, err := UserScopeEnv(home)
	if err != nil {
		t.Fatalf("UserScopeEnv: %v", err)
	}
	for key, want := range map[string]string{
		"PIXI_CACHE_DIR":    filepath.Join(override, "pixi"),
		"RATTLER_CACHE_DIR": filepath.Join(override, "rattler"),
		"NPM_CONFIG_PREFIX": filepath.Join(home, ".npm-global"),
		"CARGO_HOME":        filepath.Join(home, ".cargo"),
	} {
		if got := env[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestUserScopeBindMountsFollowTheCacheRoot is the R7 control for the bind-mount
// half of the builder migration: the pre-change code mounted $HOME/.cache/charly
// and ignored the override, so the override key is absent on that tree.
func TestUserScopeBindMountsFollowTheCacheRoot(t *testing.T) {
	home := t.TempDir()
	override := filepath.Join(t.TempDir(), "relocated-cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("CHARLY_CACHE_DIR", override)

	mounts, err := UserScopeBindMounts(home)
	if err != nil {
		t.Fatalf("UserScopeBindMounts: %v", err)
	}
	if _, ok := mounts[override]; !ok {
		t.Errorf("the cache root %q is not bind-mounted; mounts = %v", override, mounts)
	}
	for _, sub := range []string{".pixi", ".cargo", ".npm-global"} {
		path := filepath.Join(home, sub)
		if _, ok := mounts[path]; !ok {
			t.Errorf("the standard subdir %q is not bind-mounted; mounts = %v", path, mounts)
		}
	}
	if len(mounts) != 4 {
		t.Errorf("len(mounts) = %d, want 4 (three HOME subdirs + the cache root)", len(mounts))
	}
	if _, err := os.Stat(override); err != nil {
		t.Errorf("the cache root was not created: %v", err)
	}
}
