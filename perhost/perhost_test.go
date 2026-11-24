package perhost

// perhost_test.go — the ONE per-host config writer's contract, exercised the way the real
// commands use it (several callers, each touching only its own key, some of them concurrently).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: v} }

func TestUpdatePreservesEveryOtherSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	seed := "ollama-cdi:\n  pod:\n    image: ollama-cdi\nledger:\n  deploys:\n    x: applied\nsystem:\n  hostname: h\ncache:\n  git:\n    bypass: false\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The ref cache writes ITS section through the shared writer.
	if err := Update(path, func(root *yaml.Node) (bool, error) {
		return Set(root, scalar("true"), "cache", "git", "bypass"), nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file must still exist: %v", err)
	}
	s := string(data)
	for _, want := range []string{"ollama-cdi:", "ledger:", "deploys:", "system:", "hostname:", "cache:"} {
		if !strings.Contains(s, want) {
			t.Errorf("Update dropped the %q section:\n%s", want, s)
		}
	}
	if !strings.Contains(s, "bypass: true") {
		t.Errorf("the caller's own key was not written:\n%s", s)
	}
}

// A corrupt file must be REFUSED, never truncated: the writer calls no mutation and leaves the
// bytes for recovery.
func TestUpdateRefusesACorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	corrupt := "ollama-cdi: [unclosed\n"
	if err := os.WriteFile(path, []byte(corrupt), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	called := false
	err := Update(path, func(root *yaml.Node) (bool, error) {
		called = true
		return true, nil
	})
	if err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if called {
		t.Fatal("mutate must not run on a corrupt file")
	}
	data, _ := os.ReadFile(path)
	if string(data) != corrupt {
		t.Fatalf("the corrupt bytes must be left untouched, got:\n%s", data)
	}
}

// The real shape: several commands writing DIFFERENT sections at the same time. Every key must
// survive, and the file must always parse — the contract that broke on a real host on 2026-11-24.
func TestConcurrentUpdatesPreserveEachOthersKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	if err := os.WriteFile(path, []byte("base:\n  keep: me\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("section%d", i)
			err := Update(path, func(root *yaml.Node) (bool, error) {
				Set(root, scalar(fmt.Sprintf("value%d", i)), key, "leaf")
				return true, nil
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Update: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if uerr := yaml.Unmarshal(data, &doc); uerr != nil {
		t.Fatalf("the file must parse after concurrent writes: %v\n%s", uerr, data)
	}
	if _, ok := doc["base"]; !ok {
		t.Errorf("the pre-existing key was dropped:\n%s", data)
	}
	for i := 0; i < writers; i++ {
		key := fmt.Sprintf("section%d", i)
		if _, ok := doc[key]; !ok {
			t.Errorf("concurrent writer %d lost its own key %q:\n%s", i, key, data)
		}
	}
}

// A key that exists but is NOT a mapping must be REPLACED in place when a nested set walks
// through it: appending would emit a second key of the same name, which parsers resolve
// differently depending on which one wins. The reviewer of the shared-writer PR flagged this as
// a latent path while the package is being installed as THE writer for every config section.
func TestSetReplacesANonMappingIntermediateInsteadOfDuplicatingTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "charly.yml")
	if err := os.WriteFile(path, []byte("cache: not-a-mapping\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := Update(path, func(root *yaml.Node) (bool, error) {
		return Set(root, scalar("true"), "cache", "git", "bypass"), nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n := strings.Count(string(data), "cache:"); n != 1 {
		t.Fatalf("Set emitted %d `cache:` keys, want 1 — a duplicate key is the bug:\n%s", n, data)
	}
	var doc map[string]any
	if uerr := yaml.Unmarshal(data, &doc); uerr != nil {
		t.Fatalf("the file must parse: %v\n%s", uerr, data)
	}
	cache, ok := doc["cache"].(map[string]any)
	if !ok {
		t.Fatalf("cache must now be a mapping, got %T:\n%s", doc["cache"], data)
	}
	git, ok := cache["git"].(map[string]any)
	if !ok {
		t.Fatalf("the nested mapping was not written:\n%s", data)
	}
	// The scalar node is written verbatim, so YAML decodes an unquoted `true` as a bool.
	switch v := git["bypass"].(type) {
	case bool:
		if !v {
			t.Fatalf("bypass must be true, got false:\n%s", data)
		}
	case string:
		if v != "true" {
			t.Fatalf("bypass must be true, got %q:\n%s", v, data)
		}
	default:
		t.Fatalf("bypass has an unexpected type %T:\n%s", git["bypass"], data)
	}
}
