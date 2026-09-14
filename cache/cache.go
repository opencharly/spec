// Package cache provides the ONE shared persistent-cache mechanism for every
// charly cache (R3): the image list, the image labels, the submodule verdicts,
// the guest-probe results, the resolved-project envelope, and the materialized
// loader tree all read-file → unmarshal → content-check → miss and write
// atomically through this package.
//
// DOCKER-LIKE CONTENT ADDRESSING (no TTL). An entry is keyed by the CONTENT of
// its inputs — `Key(components...)` hashes the ordered input components into a
// stable digest (the Docker layer-digest model) — and it is VALID WHILE THAT
// KEY IS PRESENT. There is NO time validity anywhere: a cached value is served
// for as long as its input content is unchanged, however old. When an input
// changes, its content hash changes, the key changes, and the caller naturally
// misses → re-fetches → stores under the new key. Reclamation is a STORAGE
// concern ONLY (the bounded entry count, oldest write first) — it NEVER decides
// validity.
//
// This replaces the former TTL model. TTL was a correctness hazard, not just
// latency: a cache keyed on a coarse input (e.g. the top-level charly.yml only)
// with a 5-minute timer served STALE data after a finer input (a generated
// pr-beds/ tree) changed, for up to the whole TTL. Content addressing removes
// the class: a changed input is a new key, immediately.
//
// The callers compute the components. The invalidation rule is local and
// explicit: a component IS the content that determines the value, so a change
// in that content is a new key. Callers that mutate the underlying store
// (build/pull) may also call Invalidate for an immediate eviction, but that is
// belt-and-braces — the content key is the primary mechanism.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxEntriesDefault bounds a single cache file (the Docker builder
// --keep-storage analogue). The write path reclaims the oldest entries beyond
// the cap — storage-bound ONLY, never a validity input. A package var so
// callers/tests can size it per cache.
var MaxEntriesDefault = 64

// File is the on-disk cache shape: key -> entry. The key is a content address
// (Key(...)); the entry carries only the value and its write time (reclamation
// ordering). There is deliberately NO expiry timestamp — validity is the key.
type File struct {
	Entries map[string]Entry `json:"entries"`
}

// Entry is one cached value + when it was written (RECLAMATION ordering only;
// never a validity input).
type Entry struct {
	Value   json.RawMessage `json:"value"`
	Written time.Time       `json:"written"`
}

// Key returns the content address for an input: the hex SHA-256 over the
// NUL-separated ordered components (the Docker layer-digest model). The same
// components in the same order always produce the same key; any change in any
// component produces a different key. Callers pass the content that the cached
// value is a FUNCTION of (never a wall-clock or a random).
func Key(components ...string) string {
	h := sha256.New()
	for i, c := range components {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(c))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Read returns the cached value for key, decoding it into out. Validity is the
// key's presence — there is NO TTL. Returns false on a miss (absent file,
// corrupt entry, or a decode error); callers then recompute + Write.
func Read(path, key string, out any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cf File
	if json.Unmarshal(data, &cf) != nil {
		return false
	}
	e, ok := cf.Entries[key]
	if !ok {
		return false
	}
	if json.Unmarshal(e.Value, out) != nil {
		return false
	}
	return true
}

// Write persists value under key in the cache file, atomically (tempfile +
// rename). Best-effort: a write failure is silent (the cache is an optimization,
// never a correctness dependency). The entry is stored with the write time
// (reclamation ordering only) and the file is pruned to MaxEntriesDefault.
func Write(path, key string, value any) {
	WriteMax(path, key, value, MaxEntriesDefault)
}

// WriteMax is Write with an explicit entry bound.
func WriteMax(path, key string, value any, max int) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	var cf File
	if data, rerr := os.ReadFile(path); rerr == nil {
		_ = json.Unmarshal(data, &cf)
	}
	if cf.Entries == nil {
		cf.Entries = map[string]Entry{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	cf.Entries[key] = Entry{Value: raw, Written: time.Now()}
	prune(cf.Entries, max)
	data, err := json.Marshal(cf)
	if err != nil {
		return
	}
	// Atomic publish: a concurrent lock-free reader sees either the complete old
	// file or the complete new one, never a torn one (a unique temp keeps even
	// same-path writers safe).
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return
	}
	tmp := f.Name()
	_ = f.Chmod(0o644)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

// prune reclaims storage Docker-style: when the entry count exceeds max, the
// OLDEST entries (by write time) are removed. This is RECLAMATION — it never
// invalidates a live entry, because validity is the content key (a pruned entry
// is only ever an orphan of an input that no longer exists).
func prune(entries map[string]Entry, max int) {
	if max <= 0 || len(entries) <= max {
		return
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return entries[keys[i]].Written.Before(entries[keys[j]].Written)
	})
	for i := 0; i < len(keys)-max; i++ {
		delete(entries, keys[i])
	}
}

// Invalidate removes the cache file (an explicit eviction for a mutating action
// such as a build/pull). Belt-and-braces: the content key is the primary
// mechanism; this is only for an immediate clear.
func Invalidate(path string) {
	_ = os.Remove(path)
}

// HashString is the content address of a single string (a convenience over Key
// for the many callers whose input is one blob of content).
func HashString(s string) string { return Key(s) }

// HashStrings hashes an ordered list of strings (a convenience wrapper).
func HashStrings(comp ...string) string { return Key(comp...) }

var _ = strings.Join
