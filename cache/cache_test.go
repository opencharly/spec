package cache

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cache_test.go — the generalized Store. Each test FAILS without its behavior:
// the three validity modes, the Fill double-check, atomic publication, prune.

func TestTTLValidity(t *testing.T) {
	s := Open(t.TempDir())
	s.WriteValue("k", "v")
	var got string
	if !s.ReadTTL("k", time.Minute, &got) || got != "v" {
		t.Fatalf("fresh read = %q", got)
	}
	// Backdate past the TTL → miss.
	e, _ := s.Get("k")
	e.Resolved = time.Now().Add(-2 * time.Minute)
	s.PutEntry("k", e)
	if s.ReadTTL("k", time.Minute, &got) {
		t.Fatal("stale entry must miss")
	}
}

func TestComponentsValidity(t *testing.T) {
	s := Open(t.TempDir())
	e := Entry{Value: json.RawMessage(`"v"`), Components: map[string]string{"sha": "abc", "repo": "o/r"}}
	s.Put("k", e)
	got, ok := s.Get("k")
	if !ok || !got.FreshComponents(map[string]string{"sha": "abc", "repo": "o/r"}) {
		t.Fatal("matching components must be fresh")
	}
	if got.FreshComponents(map[string]string{"sha": "def", "repo": "o/r"}) {
		t.Fatal("drifted component must be stale")
	}
	if got.FreshComponents(map[string]string{"sha": "abc"}) {
		t.Fatal("a different component count must be stale")
	}
}

func TestValidatorRoundTrip(t *testing.T) {
	s := Open(t.TempDir())
	s.Put("k", Entry{Value: json.RawMessage(`"v"`), Validator: `W/"etag-1"`})
	got, ok := s.Get("k")
	if !ok || got.Validator != `W/"etag-1"` {
		t.Fatalf("validator = %q", got.Validator)
	}
}

func TestFillComputesOnceUnderConcurrency(t *testing.T) {
	s := Open(t.TempDir())
	var calls atomic.Int32
	const workers = 16
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Fill("k", func() (Entry, error) {
				calls.Add(1)
				time.Sleep(2 * time.Millisecond) // widen the contention window
				return Entry{Value: json.RawMessage(`"computed"`)}, nil
			})
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("fill ran %d times, want 1 (the per-key flock must serialize first-missers)", n)
	}
	var got string
	if !s.Read("k", &got) || got != "computed" {
		t.Fatalf("cached value = %q", got)
	}
}

func TestFillReturnsExistingWithoutCallingFill(t *testing.T) {
	s := Open(t.TempDir())
	s.WriteValue("k", "existing")
	called := false
	e, err := s.Fill("k", func() (Entry, error) {
		called = true
		return Entry{}, nil
	})
	if err != nil || called {
		t.Fatalf("fill must not run on a hit (called=%v err=%v)", called, err)
	}
	var got string
	if !e.Decode(&got) || got != "existing" {
		t.Fatalf("value = %q", got)
	}
}

func TestPruneReclaimsOldest(t *testing.T) {
	s := OpenLimited(t.TempDir(), 3)
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		// Space the write times so ordering is deterministic.
		s.Put(k, Entry{Value: json.RawMessage(`1`)})
		time.Sleep(time.Millisecond)
	}
	if s.Len() > 3 {
		t.Fatalf("store holds %d entries, want <= 3", s.Len())
	}
	if _, ok := s.Get("a"); ok {
		t.Fatal("the oldest entry must be reclaimed")
	}
	if _, ok := s.Get("e"); !ok {
		t.Fatal("the newest entry must survive")
	}
}

func TestInertStoreNeverErr(t *testing.T) {
	s := Open("")
	if _, ok := s.Get("k"); ok {
		t.Fatal("inert store must miss")
	}
	s.WriteValue("k", "v") // no-op, no panic
	if s.Len() != 0 {
		t.Fatal("inert store must stay empty")
	}
	e, err := s.Fill("k", func() (Entry, error) {
		return Entry{Value: json.RawMessage(`"inline"`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if !e.Decode(&got) || got != "inline" {
		t.Fatalf("inert Fill must return the computed value, got %q", got)
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	s := Open(t.TempDir())
	s.WriteValue("k", "v")
	if _, ok := s.Get("k"); !ok {
		t.Fatal("entry missing after write")
	}
	if err := s.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("entry must be gone after Delete")
	}
	// Deleting an absent key is a no-op, never an error.
	if err := s.Delete("absent"); err != nil {
		t.Fatalf("Delete of an absent key must not error: %v", err)
	}
}

func TestOpenNamedResolvesUnderRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHARLY_CACHE_DIR", root)
	s := OpenNamed("things")
	if got, want := s.Dir(), root+"/things"; got != want {
		t.Fatalf("OpenNamed dir = %q, want %q", got, want)
	}
	s.WriteValue("k", "v")
	if _, ok := OpenNamed("things").Get("k"); !ok {
		t.Fatal("OpenNamed must reopen the SAME store on disk")
	}
}

func TestHashHex(t *testing.T) {
	if HashHex("abc") == HashHex("abd") {
		t.Fatal("HashHex must be content-sensitive")
	}
	if HashHex("same") != HashHex("same") {
		t.Fatal("HashHex must be deterministic")
	}
	if len(HashHex("x")) != 64 {
		t.Fatalf("HashHex must be a hex sha256 (64 chars), got %d", len(HashHex("x")))
	}
}

func TestKeyDigestStableAndOrdered(t *testing.T) {
	a := KeyDigest(map[string]string{"x": "1", "y": "2"})
	b := KeyDigest(map[string]string{"y": "2", "x": "1"})
	if a != b {
		t.Fatal("KeyDigest must be order-independent")
	}
	if a == KeyDigest(map[string]string{"x": "1", "y": "3"}) {
		t.Fatal("KeyDigest must change when a component changes")
	}
}
