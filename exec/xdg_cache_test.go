package exec

import (
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
