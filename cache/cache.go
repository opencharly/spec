// Package cache provides the ONE shared persistent-cache mechanism for the
// status hot path (R3 — the image list, the image labels, and the submodule
// verdicts all use the same read-file → unmarshal → TTL-check → miss path and
// the same atomic write). The caches live under the charly dir
// (~/.config/charly/cache/), keyed by a content key, with a TTL policy.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// File is the on-disk cache shape: key -> entry (value + resolution time).
type File struct {
	Entries map[string]Entry `json:"entries"`
}

// Entry is one cached value + when it was resolved (RFC3339).
type Entry struct {
	Value    json.RawMessage `json:"value"`
	Resolved time.Time       `json:"resolved"`
}

// Read returns the cached value for key if fresh (within ttl), decoding it into
// out. Returns false on a miss (absent, corrupt, stale, or a decode error).
func Read(path, key string, ttl time.Duration, out any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cf File
	if json.Unmarshal(data, &cf) != nil {
		return false
	}
	e, ok := cf.Entries[key]
	if !ok || time.Since(e.Resolved) > ttl {
		return false
	}
	if json.Unmarshal(e.Value, out) != nil {
		return false
	}
	return true
}

// Write persists value under key in the cache file, atomically (tempfile +
// rename). Best-effort: a write failure is silent (the cache is an
// optimization, never a correctness dependency).
func Write(path, key string, value any) {
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
	cf.Entries[key] = Entry{Value: raw, Resolved: time.Now()}
	data, err := json.Marshal(cf)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// Invalidate removes the cache file (the build/deploy commands call this when
// a new image is created, so the next status run re-fetches the fresh list).
func Invalidate(path string) {
	_ = os.Remove(path)
}
