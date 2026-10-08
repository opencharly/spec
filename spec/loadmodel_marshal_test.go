package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

// loadmodel_marshal_test.go — the R7 regression for charly#847, replacing the throwaway probe
// that demonstrated the mechanism but shipped no coverage.
//
// WHY THIS IS NOT A SYNTHETIC CYCLE. The namespace graph is cyclic and shared BY DESIGN:
// `walkNamespace` (sdk/loaderkit/walk.go) returns an in-progress node BEFORE recursing so the host
// materialize can emit a REFERENCE mount and preserve POINTER IDENTITY across it — which makes a
// namespace reachable from itself. Every other traversal of that graph carries a cycle guard (an
// ancestor stack); `json.Marshal` cannot, and the marshaller is on a load path:
// `sdk/loaderkit/load_cache.go` computes the cache key with `json.Marshal(lp)` → `HashHex`.
//
// Before this fix a linked-worktree load died here, with exactly this error:
//
//	json: unsupported value: encountered a cycle via *spec.UnifiedFile
//
// The two fields this test guards are HOST-INTERNAL (never authored, never on the wire), which is
// why they leave the marshal surface rather than being given a cycle-tolerant encoder.

// TestUnifiedFileMarshalSurvivesACyclicNamespaceGraph is the discriminating arm: marshal a graph
// whose two namespaces reference each other, and require the marshal to SUCCEED. On the pre-fix
// tags (Namespaces carrying only `yaml:"-"`) this fails with the cycle error above.
func TestUnifiedFileMarshalSurvivesACyclicNamespaceGraph(t *testing.T) {
	a, b := &UnifiedFile{Version: "1"}, &UnifiedFile{Version: "1"}
	a.Namespaces = map[string]*UnifiedFile{"b": b}
	b.Namespaces = map[string]*UnifiedFile{"a": a} // the cycle the real graph has

	out, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("json.Marshal over a cyclic namespace graph must succeed (the load cache key is computed this way): %v", err)
	}
	if len(out) == 0 {
		t.Fatal("marshal produced no output")
	}
}

// TestUnifiedFileMarshalOmitsHostInternalFields is the other half, and it is the reason the fix is
// `json:"-"` rather than a cycle-tolerant encoder: the host-internal fields must not appear in the
// marshalled projection AT ALL. Namespaces and RootDir are populated by the materialize pass and
// are never authored or carried on the wire; Manifest is the deliberate exception (it rides the
// opaque candy-map fold and is json-tagged `__manifest`), and is asserted to prove the test is not
// merely matching an absent string.
func TestUnifiedFileMarshalOmitsHostInternalFields(t *testing.T) {
	u := &UnifiedFile{Version: "1"}
	u.RootDir = "/some/root/dir"
	u.Namespaces = map[string]*UnifiedFile{"child": {Version: "1", RootDir: "/some/child/dir"}}

	out, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(out)
	for _, key := range []string{`"Namespaces"`, `"RootDir"`} {
		if strings.Contains(s, key) {
			t.Errorf("host-internal field %s leaked into the marshalled projection: %s", key, s)
		}
	}
	// The root dir VALUE must not leak either, under any key spelling: it is a host path, and the
	// cache key it feeds is a content hash, not a location hash.
	if strings.Contains(s, "/some/root/dir") {
		t.Errorf("RootDir's value leaked into the marshalled projection: %s", s)
	}
	// Control: the encoder IS emitting keys, so an absent `"RootDir"` above is the field being
	// omitted and not the whole struct collapsing to `{}` or an error.
	if !strings.Contains(s, `"version"`) {
		t.Fatalf("control failed: the marshalled projection carries no version key at all: %s", s)
	}
}
