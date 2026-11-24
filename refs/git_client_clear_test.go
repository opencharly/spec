package refs

// git_client_clear_test.go — `charly cache clear/refresh` must clear the CACHE SECTION of
// the per-host file, never the file itself.
//
// The per-host file (~/.config/charly/charly.yml) is a SHARED document: the deploy,
// provides, ledger and system entries live in it beside `cache:`. ClearCache used to call
// os.Remove on it, so `charly cache refresh` wiped a host's deployments and the next
// resolution's save() rebuilt the file with nothing but `cache:` — measured on a real
// host on 2026-11-24, where every deployed container's config entry vanished while the
// containers kept running. save() already rewrites only the cache section and preserves
// every other key, which is what makes clearing the same write with empty maps.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearCachePreservesTheDeployConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "charly.yml")
	seed := `ollama-cdi:
  pod:
    image: ollama-cdi
    env:
      OLLAMA_PULL_MODELS: "qwen3-vl:2b-instruct-q4_K_M"
provides:
  env:
    - name: OLLAMA_HOST
      value: http://charly-ollama-cdi:11434
ledger:
  deploys:
    ollama-cdi: applied
cache:
  git:
    bypass: false
    latest_tags:
      https://github.com/opencharly/layer-supervisord.git:
        value: v2026.314.0710
        resolved: 2026-11-24T00:00:00Z
`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := NewGitClient(path)
	if err := g.ClearCache(); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the per-host config must still exist after a cache clear: %v", err)
	}
	s := string(got)
	for _, want := range []string{"ollama-cdi:", "OLLAMA_PULL_MODELS", "provides:", "OLLAMA_HOST", "ledger:", "deploys:"} {
		if !strings.Contains(s, want) {
			t.Errorf("ClearCache dropped %q from the per-host config:\n%s", want, s)
		}
	}
	// The cache section must be CLEARED: the key stays (save() always writes it) but no
	// entry may remain. Assert on the ENTRY, never on the key name.
	if strings.Contains(s, "layer-supervisord") || strings.Contains(s, "v2026.314.0710") {
		t.Errorf("the cache section must be cleared, but it still holds entries:\n%s", s)
	}

	// And the cache really is cleared for the next resolution: a fresh client starts empty.
	fresh := NewGitClient(path)
	if _, n := fresh.CacheStatus(); n != 0 {
		t.Errorf("a fresh client after ClearCache reports %d cached answer(s), want 0", n)
	}
}
