package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// cache_test.go — the OCI-layout ArtifactStore. Each test FAILS without its
// behavior: the three validity modes, the Fill double-check, the on-disk OCI
// Image Layout shape, atomic publication, and GC.

// putRaw stores e under key preserving Resolved (the backdating seam).
func putRaw(t *testing.T, l *Layout, key string, e Entry) {
	t.Helper()
	if err := l.put(key, e); err != nil {
		t.Fatalf("put %q: %v", key, err)
	}
}

func TestTTLValidity(t *testing.T) {
	l := OpenLayout(t.TempDir())
	if err := l.Put("k", Entry{Payload: []byte(`"v"`)}); err != nil {
		t.Fatal(err)
	}
	e, ok := l.Get("k")
	if !ok || !e.FreshTTL(time.Minute) || string(e.Payload) != `"v"` {
		t.Fatalf("fresh read = %+v ok=%v", e, ok)
	}
	// Backdate past the TTL → stale.
	putRaw(t, l, "k", Entry{Payload: []byte(`"v"`), Resolved: time.Now().Add(-2 * time.Minute)})
	e, _ = l.Get("k")
	if e.FreshTTL(time.Minute) {
		t.Fatal("stale entry must not be fresh")
	}
}

func TestComponentsValidity(t *testing.T) {
	l := OpenLayout(t.TempDir())
	if err := l.Put("k", Entry{Payload: []byte(`"v"`), Components: map[string]string{"sha": "abc", "repo": "o/r"}}); err != nil {
		t.Fatal(err)
	}
	e, ok := l.Get("k")
	if !ok || !e.FreshComponents(map[string]string{"sha": "abc", "repo": "o/r"}) {
		t.Fatal("matching components must be fresh")
	}
	if e.FreshComponents(map[string]string{"sha": "def", "repo": "o/r"}) {
		t.Fatal("drifted component must be stale")
	}
	if e.FreshComponents(map[string]string{"sha": "abc"}) {
		t.Fatal("a different component count must be stale")
	}
}

func TestValidatorRoundTrip(t *testing.T) {
	l := OpenLayout(t.TempDir())
	if err := l.Put("k", Entry{Payload: []byte(`"v"`), Validator: `W/"etag-1"`}); err != nil {
		t.Fatal(err)
	}
	e, ok := l.Get("k")
	if !ok || e.Validator != `W/"etag-1"` || !e.FreshValidator(`W/"etag-1"`) {
		t.Fatalf("validator = %q", e.Validator)
	}
	if e.FreshValidator(`W/"etag-2"`) {
		t.Fatal("a changed validator must be stale")
	}
}

// TestOnDiskIsOCILayout locks the storage contract: the store directory is a
// valid OCI Image Layout (oci-layout marker + index.json + content-addressed
// blobs). This is what makes registry push/pull possible at all.
func TestOnDiskIsOCILayout(t *testing.T) {
	dir := t.TempDir()
	l := OpenLayout(dir)
	if err := l.Put("k", Entry{Payload: []byte(`{"hello":"world"}`)}); err != nil {
		t.Fatal(err)
	}
	// oci-layout marker with the required field.
	marker, err := os.ReadFile(filepath.Join(dir, ociv1.ImageLayoutFile))
	if err != nil {
		t.Fatalf("missing oci-layout marker: %v", err)
	}
	var layout ociv1.ImageLayout
	if json.Unmarshal(marker, &layout) != nil || layout.Version != ociv1.ImageLayoutVersion {
		t.Fatalf("bad oci-layout marker: %s", marker)
	}
	// index.json is a valid image index naming the entry.
	idxBytes, err := os.ReadFile(filepath.Join(dir, ociv1.ImageIndexFile))
	if err != nil {
		t.Fatalf("missing index.json: %v", err)
	}
	var idx ociv1.Index
	if json.Unmarshal(idxBytes, &idx) != nil || len(idx.Manifests) != 1 {
		t.Fatalf("bad index.json: %s", idxBytes)
	}
	// Every referenced blob exists at blobs/<alg>/<encoded> AND hashes to its name.
	for _, d := range idx.Manifests {
		if got := d.Annotations[AnnotationCacheKey]; got != "k" {
			t.Fatalf("manifest annotation key = %q, want k", got)
		}
		body, rerr := os.ReadFile(l.blobPath(d.Digest))
		if rerr != nil {
			t.Fatalf("manifest blob %s missing: %v", d.Digest, rerr)
		}
		var m ociv1.Manifest
		if json.Unmarshal(body, &m) != nil {
			t.Fatalf("manifest blob not a manifest: %s", body)
		}
		for _, bd := range append([]ociv1.Descriptor{m.Config}, m.Layers...) {
			blob, berr := os.ReadFile(l.blobPath(bd.Digest))
			if berr != nil {
				t.Fatalf("blob %s missing: %v", bd.Digest, berr)
			}
			if int64(len(blob)) != bd.Size {
				t.Fatalf("blob %s size = %d, want %d", bd.Digest, len(blob), bd.Size)
			}
			if bd.MediaType == MediaTypeCacheConfig {
				var cfg entryConfig
				if json.Unmarshal(blob, &cfg) != nil {
					t.Fatalf("config blob not decodable: %s", blob)
				}
			}
		}
	}
}

func TestPutIsContentAddressedAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	l := OpenLayout(dir)
	// Two keys with identical payloads share the payload blob (CAS dedup).
	if err := l.Put("a", Entry{Payload: []byte("same")}); err != nil {
		t.Fatal(err)
	}
	if err := l.Put("b", Entry{Payload: []byte("same")}); err != nil {
		t.Fatal(err)
	}
	blobsRoot := filepath.Join(dir, ociv1.ImageBlobsDir)
	var payloadCount int
	_ = filepath.WalkDir(blobsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if string(b) == "same" {
			payloadCount++
		}
		return nil
	})
	if payloadCount != 1 {
		t.Fatalf("identical payloads stored %d times, want 1 (content addressing)", payloadCount)
	}
}

func TestFillComputesOnceUnderConcurrency(t *testing.T) {
	l := OpenLayout(t.TempDir())
	var calls atomic.Int32
	const workers = 16
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = l.Fill("k", func() (Entry, error) {
				calls.Add(1)
				time.Sleep(2 * time.Millisecond) // widen the contention window
				return Entry{Payload: []byte(`"computed"`)}, nil
			})
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("fill ran %d times, want 1 (the per-key flock must serialize first-missers)", n)
	}
	e, ok := l.Get("k")
	if !ok || string(e.Payload) != `"computed"` {
		t.Fatalf("cached value = %q ok=%v", e.Payload, ok)
	}
}

func TestFillReturnsExistingWithoutCallingFill(t *testing.T) {
	l := OpenLayout(t.TempDir())
	if err := l.Put("k", Entry{Payload: []byte(`"existing"`)}); err != nil {
		t.Fatal(err)
	}
	called := false
	e, err := l.Fill("k", func() (Entry, error) {
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
	l := OpenLayoutLimited(t.TempDir(), 3)
	base := time.Now().Add(-time.Hour)
	for i, k := range []string{"a", "b", "c", "d", "e"} {
		putRaw(t, l, k, Entry{Payload: []byte(`1`), Resolved: base.Add(time.Duration(i) * time.Minute)})
	}
	if l.Len() > 3 {
		t.Fatalf("store holds %d entries, want <= 3", l.Len())
	}
	if _, ok := l.Get("a"); ok {
		t.Fatal("the oldest entry must be reclaimed")
	}
	if _, ok := l.Get("e"); !ok {
		t.Fatal("the newest entry must survive")
	}
}

func TestInertStoreNeverErr(t *testing.T) {
	l := OpenLayout("")
	if _, ok := l.Get("k"); ok {
		t.Fatal("inert store must miss")
	}
	if err := l.Put("k", Entry{Payload: []byte(`"v"`)}); err != nil {
		t.Fatalf("inert Put must not error: %v", err)
	}
	if l.Len() != 0 {
		t.Fatal("inert store must stay empty")
	}
	e, err := l.Fill("k", func() (Entry, error) {
		return Entry{Payload: []byte(`"inline"`)}, nil
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
	l := OpenLayout(t.TempDir())
	if err := l.Put("k", Entry{Payload: []byte(`"v"`)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := l.Get("k"); !ok {
		t.Fatal("entry missing after write")
	}
	if err := l.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := l.Get("k"); ok {
		t.Fatal("entry must be gone after Delete")
	}
	if err := l.Delete("absent"); err != nil {
		t.Fatalf("Delete of an absent key must not error: %v", err)
	}
}

// TestGCReclaimsUnreferencedBlobs proves GC drops an orphaned blob (written by a
// superseded entry) while keeping the live one.
func TestGCReclaimsUnreferencedBlobs(t *testing.T) {
	dir := t.TempDir()
	l := OpenLayout(dir)
	if err := l.Put("k", Entry{Payload: []byte("old")}); err != nil {
		t.Fatal(err)
	}
	if err := l.Put("k", Entry{Payload: []byte("new")}); err != nil {
		t.Fatal(err)
	}
	blobsRoot := filepath.Join(dir, ociv1.ImageBlobsDir)
	hasBlob := func(want string) bool {
		found := false
		_ = filepath.WalkDir(blobsRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			b, _ := os.ReadFile(path)
			if string(b) == want {
				found = true
			}
			return nil
		})
		return found
	}
	if !hasBlob("old") {
		t.Fatal("precondition: superseded payload blob should exist before GC")
	}
	if err := l.GC(); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if hasBlob("old") {
		t.Fatal("GC must reclaim the unreferenced payload blob")
	}
	if !hasBlob("new") {
		t.Fatal("GC must keep the live payload blob")
	}
}

func TestOpenNamedLayoutResolvesUnderRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHARLY_CACHE_DIR", root)
	l := OpenNamedLayout("things")
	if got, want := l.Dir(), filepath.Join(root, "things"); got != want {
		t.Fatalf("OpenNamedLayout dir = %q, want %q", got, want)
	}
	if err := l.Put("k", Entry{Payload: []byte(`"v"`)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := OpenNamedLayout("things").Get("k"); !ok {
		t.Fatal("OpenNamedLayout must reopen the SAME store on disk")
	}
}

// TestHashHex pins the content-addressed digest helper: content-sensitive,
// deterministic (verified by re-deriving a fresh value), and a 64-char hex sha256.
func TestHashHex(t *testing.T) {
	if HashHex("abc") == HashHex("abd") {
		t.Fatal("HashHex must be content-sensitive")
	}
	if got, again := HashHex("same"), HashHex("same"); got != again {
		t.Fatalf("HashHex must be deterministic: %s vs %s", got, again)
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

// TestFillStampsResolved locks the Fill write-time contract: the compute-once
// path stamps Resolved to NOW (like Put), so a fill callback returning a zero
// Resolved is NOT persisted as instantly-stale. Without the stamp, a TTL caller's
// freshly-computed entry reads as a miss (defeating compute-once) and sorts as
// the OLDEST entry for reclamation.
func TestFillStampsResolved(t *testing.T) {
	l := OpenLayout(t.TempDir())
	if _, err := l.Fill("k", func() (Entry, error) {
		// A zero-Resolved entry — the shape a degrade/empty callback returns.
		return Entry{Payload: []byte(`"v"`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	e, ok := l.Get("k")
	if !ok {
		t.Fatal("entry missing after Fill")
	}
	if !e.FreshTTL(time.Minute) {
		t.Fatalf("Fill must stamp Resolved=now so the entry is TTL-fresh; got Resolved=%v", e.Resolved)
	}
}
