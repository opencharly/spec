// Package cache provides the ONE shared persistent-cache mechanism (R3): a
// keyed Store with three interchangeable VALIDITY modes and one storage layout.
//
// A Store is a directory of one JSON entry per key, keyed by a content key, with
// the entry recording the validity inputs so the READ decides freshness. This
// package's own consumers — spec/refs (submodule verdicts) and spec/container
// (image list, image labels) — are Stores today; the loader's materialized-tree
// cache (sdk/loaderkit) and a plugin HTTP client (plugin-gh) are folded onto the
// SAME Store in their own producer-ordered changes, so a feature never grows its
// own bespoke cache.
//
// Validity lives IN the entry, never in the storage mechanism, so the same Store
// serves all three policies:
//
//   - TTL — Entry.Resolved within a window (the status hot path: the image list,
//     labels, submodule verdicts).
//   - Components — Entry.Components equal the caller's current components (the
//     content-addressed caches: the loader's materialized tree, a GitHub blob by
//     SHA). No time validity: valid while the inputs are unchanged.
//   - Validator — Entry.Validator is an upstream revalidation token (an HTTP
//     ETag / Last-Modified); the caller revalidates and refreshes on change (a
//     GitHub PR/issues read that must not re-fetch while upstream is unchanged).
//
// Layout + concurrency (the one mechanism, the spec/refs + loaderkit patterns):
// one file per key under the store directory, filenames a SHA-256 of the key (a
// key may carry '/', '|', '@' — never a safe filename). Reads are LOCK-FREE: a
// writer publishes atomically (tmp + rename), so a reader sees either the
// complete old state (miss) or the complete new one — never a torn entry. The
// MISS path takes a per-key blocking advisory flock (spec/lock) and
// double-checks under the lock, so concurrent first-missers compute ONCE and
// reuse the winner's entry. Every cache failure — a key error, a read error,
// lock contention timeout, a corrupt entry — is a MISS: the cache is an
// optimization, never a correctness dependency.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/opencharly/spec/lock"
	"github.com/opencharly/spec/spec"
)

// DefaultMaxEntries bounds a Store's size (the Docker builder --keep-storage
// analogue): beyond it the write path opportunistically reclaims the OLDEST
// entries. Reclamation is STORAGE-bound only — validity is decided by the entry
// (TTL / components / validator), never by age — so a pruned entry is only ever
// a stale-input orphan. 0 disables the cap.
const DefaultMaxEntries = 4096

// Entry is one cached value plus the inputs its validity is decided from.
//
// Value is the caller's opaque payload. Resolved is the write time (RECLAMATION
// ordering — and the TTL input where a caller uses one). Components and
// Validator are the two non-time validity inputs; a caller sets exactly the one
// its policy uses (a TTL caller sets neither).
type Entry struct {
	// Key is the ORIGINAL cache key (the filename is its hash, so the key is
	// carried inside the entry). Set by the Store on Put; a caller need not set it.
	Key string `json:"key,omitempty"`
	// Value is the cached payload (opaque JSON).
	Value json.RawMessage `json:"value"`
	// Resolved is when the value was written.
	Resolved time.Time `json:"resolved"`
	// Components are the content/identity inputs a components-validity caller
	// compares: the entry is valid iff these equal the caller's current set.
	Components map[string]string `json:"components,omitempty"`
	// Validator is an upstream revalidation token (HTTP ETag / Last-Modified) a
	// caller uses to revalidate without a body re-fetch.
	Validator string `json:"validator,omitempty"`
}

// FreshTTL reports whether the entry is within ttl of its resolution.
func (e Entry) FreshTTL(ttl time.Duration) bool {
	return time.Since(e.Resolved) <= ttl
}

// FreshComponents reports whether the entry's components equal want. Two nil
// component sets are equal; a nil/empty want matches only a nil/empty entry set.
func (e Entry) FreshComponents(want map[string]string) bool {
	if len(want) != len(e.Components) {
		return false
	}
	for k, v := range want {
		if e.Components[k] != v {
			return false
		}
	}
	return true
}

// Decode unmarshals the entry's Value into out; false on a decode error.
func (e Entry) Decode(out any) bool {
	return json.Unmarshal(e.Value, out) == nil
}

// StoreDir returns the cache DIRECTORY for a named store under the charly dir
// (~/.config/charly/cache/<name>/). CHARLY_CACHE_DIR overrides the root (the
// one override every store honors, so a caller can relocate the whole cache).
func StoreDir(name string) (string, error) {
	if root := os.Getenv("CHARLY_CACHE_DIR"); root != "" {
		return filepath.Join(root, name), nil
	}
	cfg, err := spec.DefaultDeployConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfg), "cache", name), nil
}

// Store is a persistent keyed cache rooted at a directory. Construct with Open
// and share it: the on-disk state is the point.
type Store struct {
	dir        string
	maxEntries int
}

// Open returns a Store rooted at dir (created lazily on first write), pruning to
// DefaultMaxEntries. An empty dir yields an inert store whose every operation is
// a miss/no-op — the cache is an optimization, so a caller never has to guard it.
func Open(dir string) *Store { return &Store{dir: dir, maxEntries: DefaultMaxEntries} }

// OpenNamed opens the named store under the charly cache dir
// (~/.config/charly/cache/<name>/, CHARLY_CACHE_DIR overriding the root). A
// path-resolution failure yields an inert store, so a caller never guards it —
// the ONE idiom every named-store consumer shares (R3).
func OpenNamed(name string) *Store {
	dir, err := StoreDir(name)
	if err != nil {
		return Open("")
	}
	return Open(dir)
}

// OpenLimited is Open with an explicit entry cap (0 disables pruning).
func OpenLimited(dir string, maxEntries int) *Store {
	return &Store{dir: dir, maxEntries: maxEntries}
}

// Dir reports the store root ("" for an inert store).
func (s *Store) Dir() string { return s.dir }

// usable reports whether the store is backed by a directory.
func (s *Store) usable() bool { return s != nil && s.dir != "" }

// entryPath is the file holding key: <dir>/<sha256(key)>.json.
func (s *Store) entryPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}

// lockPath is the per-key advisory-lock path (a locks/ sibling keeps the entries
// dir sweepable).
func (s *Store) lockPath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, "locks", hex.EncodeToString(sum[:])+".lock")
}

// Get returns the entry for key and whether it was present (and well-formed). A
// corrupt or absent entry is a miss.
func (s *Store) Get(key string) (Entry, bool) {
	if !s.usable() {
		return Entry{}, false
	}
	data, err := os.ReadFile(s.entryPath(key))
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if json.Unmarshal(data, &e) != nil {
		return Entry{}, false
	}
	return e, true
}

// Read decodes the entry for key into out, returning whether it was present and
// well-formed (the caller applies its own TTL/components/validator policy to the
// returned Entry via a companion Get — Read is the pure decode convenience).
func (s *Store) Read(key string, out any) bool {
	e, ok := s.Get(key)
	if !ok {
		return false
	}
	return e.Decode(out)
}

// ReadTTL is Read with the TTL policy applied: it decodes the entry for key into
// out only when present AND within ttl. The common status-cache read.
func (s *Store) ReadTTL(key string, ttl time.Duration, out any) bool {
	e, ok := s.Get(key)
	if !ok || !e.FreshTTL(ttl) {
		return false
	}
	return e.Decode(out)
}

// Put stores e under key, stamping Resolved to now, atomically (tmp + rename so
// a lock-free reader never sees a torn entry) and best-effort (a write failure is
// silent).
func (s *Store) Put(key string, e Entry) {
	e.Resolved = time.Now()
	s.PutEntry(key, e)
}

// PutEntry stores e under key EXACTLY as given — the caller controls Resolved
// (the backdating seam for TTL tests, and the revalidating caller that refreshes
// an unchanged entry's Resolved). Key is stamped. Atomic + best-effort like Put.
func (s *Store) PutEntry(key string, e Entry) {
	if !s.usable() {
		return
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return
	}
	e.Key = key
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	final := s.entryPath(key)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, final); err != nil {
		return
	}
	s.prune()
}

// WriteValue is Put for a caller storing a plain value under a TTL (the common
// status-cache shape): the value is marshalled and stored with no components or
// validator.
func (s *Store) WriteValue(key string, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.Put(key, Entry{Value: raw})
}

// Fill returns the entry for key, computing it with fill when absent. It
// serializes the miss under the per-key flock and DOUBLE-CHECKS under the lock,
// so concurrent first-missers compute once and reuse the winner's entry. A fill
// error is returned un-wrapped; nothing is written. fill takes no argument: the
// store-level policy is presence-or-absence, and a caller that must REVALIDATE a
// mutable upstream (the gh ETag path) reads the existing entry with Get, decides
// upstream itself, and Puts the refreshed entry — Fill is the compute-once
// primitive, not a revalidation hook.
//
// Every failure short of fill itself — an inert store, a lock timeout — degrades
// to calling fill and returning its result WITHOUT persisting: the cache never
// fails a caller.
func (s *Store) Fill(key string, fill func() (Entry, error)) (Entry, error) {
	if !s.usable() {
		return fill()
	}
	if e, ok := s.Get(key); ok {
		return e, nil
	}
	release, err := lock.AcquireFileLock(s.lockPath(key), true)
	if err != nil {
		// Lock contention (or an IO failure): compute without caching.
		return fill()
	}
	defer func() { _ = release() }()
	if e, ok := s.Get(key); ok {
		return e, nil
	}
	e, ferr := fill()
	if ferr != nil {
		return Entry{}, ferr
	}
	s.Put(key, e)
	return e, nil
}

// Delete removes key's entry (a no-op when absent).
func (s *Store) Delete(key string) error {
	if !s.usable() {
		return nil
	}
	if err := os.Remove(s.entryPath(key)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Clear drops EVERY entry in the store (the `--invalidate` / `charly cache
// clear` semantics).
func (s *Store) Clear() error {
	if !s.usable() {
		return nil
	}
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	return nil
}

// Keys returns every stored key (the original key carried in each entry), in no
// particular order. A corrupt entry is skipped.
func (s *Store) Keys() []string {
	if !s.usable() {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var keys []string
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".json" {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(s.dir, ent.Name()))
		if rerr != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil {
			continue
		}
		keys = append(keys, e.Key)
	}
	return keys
}

// Len reports the entry count.
func (s *Store) Len() int { return len(s.Keys()) }

// prune reclaims storage Docker-style: when the entry count exceeds maxEntries,
// the OLDEST entries (by Resolved) are removed. RECLAMATION only — never a
// validity input. Best-effort. A no-op when the cap is 0.
func (s *Store) prune() {
	if s.maxEntries <= 0 {
		return
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	type named struct {
		name string
		res  time.Time
	}
	var live []named
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".json" {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(s.dir, ent.Name()))
		if rerr != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil {
			continue
		}
		live = append(live, named{ent.Name(), e.Resolved})
	}
	excess := len(live) - s.maxEntries
	if excess <= 0 {
		return
	}
	sort.Slice(live, func(i, j int) bool { return live[i].res.Before(live[j].res) })
	for i := 0; i < excess; i++ {
		_ = os.Remove(filepath.Join(s.dir, live[i].name))
	}
}

// HashHex is the hex SHA-256 of s — the content-addressed digest helper shared
// by every components-keyed caller (the loader's walk envelope, a GitHub blob).
func HashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// KeyDigest is the components digest of a component set: its keys sorted and
// joined with NUL separators around "k=v" pairs, SHA-256'd. One helper so every
// components-validity caller derives the same key from the same inputs (R3).
func KeyDigest(components map[string]string) string {
	keys := make([]string, 0, len(components))
	for k := range components {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\x00", k, components[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}
