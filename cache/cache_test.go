package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readFile(t *testing.T, path string, out *File) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if json.Unmarshal(data, out) != nil {
		return false
	}
	return true
}

func writeFile(t *testing.T, path string, cf File) {
	t.Helper()
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestKeyIsContentAddressed locks the Docker-like contract: the key is a pure
// function of the ordered components. Same content -> same key (so an entry is
// served however old); any change in any component -> a different key (so a
// changed input is an immediate miss, never a stale hit).
func TestKeyIsContentAddressed(t *testing.T) {
	a := Key("repo", "branch", "sha1")
	if a != Key("repo", "branch", "sha1") {
		t.Fatal("same components must produce the same key")
	}
	if a == Key("repo", "branch", "sha2") {
		t.Fatal("a changed component must change the key")
	}
	if a == Key("branch", "repo", "sha1") {
		t.Fatal("component ORDER is part of the address")
	}
	if a == Key("repo", "branch") {
		t.Fatal("a different component COUNT must change the key")
	}
	// NUL framing: ["ab","c"] must not collide with ["a","bc"]
	if Key("ab", "c") == Key("a", "bc") {
		t.Fatal("component framing must be unambiguous")
	}
}

// TestReadServesRegardlessOfAge proves there is NO time validity: an entry
// written long ago is still a hit while its key is present.
func TestReadServesRegardlessOfAge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	key := Key("input")
	Write(path, key, map[string]string{"v": "1"})

	// Rewrite the entry's Written stamp far in the past (reclamation data only).
	var cf File
	if !readFile(t, path, &cf) {
		t.Fatal("expected the cache file")
	}
	e := cf.Entries[key]
	e.Written = e.Written.AddDate(-1, 0, 0) // one year old
	cf.Entries[key] = e
	writeFile(t, path, cf)

	var got map[string]string
	if !Read(path, key, &got) {
		t.Fatal("a present key must be served regardless of age (no TTL)")
	}
	if got["v"] != "1" {
		t.Fatalf("wrong value: %v", got)
	}
}

// TestChangedComponentIsAMiss proves the invalidation rule: a changed input is a
// new key, so the old value is NOT served.
func TestChangedComponentIsAMiss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	Write(path, Key("charly.yml@hashA"), "old")

	var out string
	if !Read(path, Key("charly.yml@hashA"), &out) || out != "old" {
		t.Fatal("the unchanged input must hit")
	}
	if Read(path, Key("charly.yml@hashB"), &out) {
		t.Fatal("a changed input (new pr-beds content) must MISS, never serve stale")
	}
}

// TestWriteIsAtomicAndPrunes proves the atomic publish (no torn read) and the
// storage-only reclamation bound.
func TestWriteIsAtomicAndPrunes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	for i := 0; i < 5; i++ {
		WriteMax(path, Key("k", string(rune('a'+i))), i, 3)
	}
	var cf File
	if !readFile(t, path, &cf) {
		t.Fatal("expected the cache file")
	}
	if len(cf.Entries) != 3 {
		t.Fatalf("expected the bound to 3 entries, got %d", len(cf.Entries))
	}
	// no temp left behind
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp-*")); len(m) != 0 {
		t.Fatalf("temp left behind: %v", m)
	}
	// the NEWEST entry survives (reclamation removes oldest)
	var out int
	if !Read(path, Key("k", string(rune('a'+4))), &out) {
		t.Fatal("the newest entry must survive the prune")
	}
}
