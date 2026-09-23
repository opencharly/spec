// Package cache provides the ONE shared persistent-cache mechanism (R3): a
// content-addressed ArtifactStore whose on-disk form is a standard OCI Image
// Layout, so every cached artifact is a real OCI manifest — and a whole named
// cache can be pushed to / pulled from an OCI registry unchanged.
//
// # The model
//
// A Store is one OCI Image Layout directory: blobs/<alg>/<digest> (content),
// index.json (the entry point), oci-layout (the version marker). Each cached
// artifact is ONE OCI image manifest:
//
//   - config blob   — the entry's validity metadata (resolved / components /
//     validator). Validity lives IN the entry, never in the storage mechanism.
//   - layers[0]     — the cached payload bytes (JSON, a tar tree, …).
//   - annotations   — the cache KEY (ai.opencharly.cache.key) and the TAG
//     (org.opencontainers.image.ref.name), both indexed by index.json.
//
// A NAMED cache is one such layout. Because it is a valid OCI layout, the
// registry transport (candy/plugin-oci's verb:oci cache-push/cache-pull) reads
// and writes it verbatim with a standard registry client — no bespoke format.
//
// # Docker cache principles, applied
//
// Content addressing (blob name = digest), a manifest/index entry point, tags
// for lookup, and no in-place mutation: a write publishes NEW blobs and swaps
// index.json atomically (tmp + rename), so a lock-free reader sees either the
// complete old index or the complete new one — never a torn entry. GC reclaims
// blobs no live manifest references.
//
// # Validity
//
//   - TTL        — Entry.Resolved within a window (the status hot path).
//   - Components — Entry.Components equal the caller's current components (the
//     content-addressed caches: the materialized tree, a GitHub blob by SHA).
//   - Validator  — Entry.Validator is an upstream revalidation token (an HTTP
//     ETag / Last-Modified); the caller revalidates and refreshes on change.
//
// Every cache failure — a read error, an unreadable blob, lock contention
// timeout, a corrupt manifest — is a MISS: the cache is an optimization, never
// a correctness dependency.
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
	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// DefaultMaxEntries bounds a Store's size (the Docker builder --keep-storage
// analogue): beyond it the write path opportunistically reclaims the OLDEST
// entries. Reclamation is STORAGE-bound only — validity is decided by the entry
// (TTL / components / validator), never by age. 0 disables the cap.
const DefaultMaxEntries = 4096

// Media types for the two blob roles an entry carries. They are namespaced
// under opencharly so the layout is self-describing without claiming to be a
// runnable container image.
const (
	// MediaTypeCacheConfig is the entry's validity-metadata config blob.
	MediaTypeCacheConfig = "application/vnd.opencharly.cache.config.v1+json"
	// MediaTypeCachePayload is the default media type of the cached payload.
	MediaTypeCachePayload = "application/vnd.opencharly.cache.payload.v1+json"
	// AnnotationCacheKey records the ORIGINAL cache key on the manifest (the
	// tag is a digest, so the key must be carried to be recoverable).
	AnnotationCacheKey = "ai.opencharly.cache.key"
)

// Entry is one cached value plus the inputs its validity is decided from.
type Entry struct {
	// Key is the ORIGINAL cache key (the manifest tag is its digest, so the key
	// is carried in an annotation). Get reconstructs it for the caller; a caller
	// need not set it on Put.
	Key string `json:"key,omitempty"`
	// Payload is the cached value's bytes (JSON, a tar tree, …).
	Payload []byte `json:"payload,omitempty"`
	// MediaType is the payload's media type. Empty means MediaTypeCachePayload.
	MediaType string `json:"media_type,omitempty"`
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

// FreshValidator reports whether the entry's validator equals the caller's
// current upstream token.
func (e Entry) FreshValidator(current string) bool { return e.Validator == current }

// Decode unmarshals the entry's Payload into out; false on a decode error.
func (e Entry) Decode(out any) bool {
	return json.Unmarshal(e.Payload, out) == nil
}

// entryConfig is the config blob of an entry manifest: the validity inputs.
type entryConfig struct {
	Resolved   time.Time         `json:"resolved"`
	Components map[string]string `json:"components,omitempty"`
	Validator  string            `json:"validator,omitempty"`
}

// ArtifactStore is the ONE content-addressed cache contract. The file-based
// Layout implements it today; the registry transport (verb:oci cache-push/pull)
// moves a whole named store to and from a registry.
type ArtifactStore interface {
	// Put stores e under key (stamping Resolved and publishing new blobs).
	Put(key string, e Entry) error
	// Get returns the entry for key and whether it was present (and well-formed).
	Get(key string) (Entry, bool)
	// Delete removes key's entry (a no-op when absent).
	Delete(key string) error
	// Keys returns every stored key.
	Keys() []string
	// Len reports the entry count.
	Len() int
	// Clear drops every entry (the `charly cache clear` semantics).
	Clear() error
	// GC reclaims blobs no live entry references and enforces the entry cap.
	GC() error
	// Dir reports the store root ("" for an inert store).
	Dir() string
}

// Layout is the simple file-based ArtifactStore: an OCI Image Layout directory.
type Layout struct {
	dir        string
	maxEntries int
}

// OpenLayout returns a Layout rooted at dir (created lazily on first write),
// pruning to DefaultMaxEntries. An empty dir yields an inert store whose every
// operation is a miss/no-op.
func OpenLayout(dir string) *Layout { return &Layout{dir: dir, maxEntries: DefaultMaxEntries} }

// OpenLayoutLimited is OpenLayout with an explicit entry cap (0 disables pruning).
func OpenLayoutLimited(dir string, maxEntries int) *Layout {
	return &Layout{dir: dir, maxEntries: maxEntries}
}

// OpenNamedLayout opens the named layout under the charly cache dir
// (~/.config/charly/cache/<name>/, CHARLY_CACHE_DIR overriding the root). A
// path-resolution failure yields an inert store, so a caller never guards it.
func OpenNamedLayout(name string) *Layout {
	dir, err := StoreDir(name)
	if err != nil {
		return OpenLayout("")
	}
	return OpenLayout(dir)
}

// StoreDir returns the cache DIRECTORY for a named store under the charly dir
// (~/.config/charly/cache/<name>/). CHARLY_CACHE_DIR overrides the root (the
// one override every store honors, so a caller can relocate the whole cache).
func StoreDir(name string) (string, error) {
	root, err := StoreRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

// StoreRoot returns the ROOT directory every named store lives under
// (~/.config/charly/cache/), honoring the CHARLY_CACHE_DIR override.
func StoreRoot() (string, error) {
	if root := os.Getenv("CHARLY_CACHE_DIR"); root != "" {
		return root, nil
	}
	cfg, err := spec.DefaultDeployConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfg), "cache"), nil
}

// NamedStores returns every named store under the cache root, sorted by name.
// A store is a directory that looks like an OCI Image Layout (carries an
// oci-layout marker) — a stray non-layout file/dir under the root is skipped,
// so a caller never mistakes unrelated cache state for an ArtifactStore. A
// missing root is an empty list, never an error.
func NamedStores() ([]string, error) {
	root, err := StoreRoot()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if _, lerr := os.Stat(filepath.Join(root, e.Name(), ociv1.ImageLayoutFile)); lerr != nil {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// Dir reports the store root ("" for an inert store).
func (l *Layout) Dir() string { return l.dir }

// usable reports whether the store is backed by a directory.
func (l *Layout) usable() bool { return l != nil && l.dir != "" }

// --- OCI layout paths ------------------------------------------------------

func (l *Layout) indexFile() string  { return filepath.Join(l.dir, ociv1.ImageIndexFile) }
func (l *Layout) layoutFile() string { return filepath.Join(l.dir, ociv1.ImageLayoutFile) }
func (l *Layout) lockFile() string   { return filepath.Join(l.dir, ".lock") }

// blobPath is the content-addressed path of a digest: <dir>/blobs/<alg>/<hex>.
func (l *Layout) blobPath(dgst digest.Digest) string {
	return filepath.Join(l.dir, ociv1.ImageBlobsDir, string(dgst.Algorithm()), dgst.Encoded())
}

// tagFor derives the OCI tag for a cache key: the hex sha256 (a valid OCI tag,
// and the same digest the legacy layout used as its filename).
func tagFor(key string) string { return HashHex(key) }

// --- blob IO ---------------------------------------------------------------

// writeBlob writes data into the blob store, returning its descriptor. Writes
// are content-addressed and idempotent: an existing blob with the same digest
// is left untouched. Publication is atomic (tmp + rename).
func (l *Layout) writeBlob(data []byte, mediaType string) (ociv1.Descriptor, error) {
	dgst := digest.FromBytes(data)
	desc := ociv1.Descriptor{MediaType: mediaType, Digest: dgst, Size: int64(len(data))}
	final := l.blobPath(dgst)
	if _, err := os.Stat(final); err == nil {
		return desc, nil
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return ociv1.Descriptor{}, err
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return ociv1.Descriptor{}, err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return ociv1.Descriptor{}, err
	}
	return desc, nil
}

// readBlob returns the bytes named by dgst (verified by the caller's decode).
func (l *Layout) readBlob(dgst digest.Digest) ([]byte, error) {
	return os.ReadFile(l.blobPath(dgst))
}

// --- index IO --------------------------------------------------------------

// readIndex returns the layout's index.json, or an empty index when absent.
func (l *Layout) readIndex() (*ociv1.Index, error) {
	data, err := os.ReadFile(l.indexFile())
	if err != nil {
		if os.IsNotExist(err) {
			return &ociv1.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ociv1.MediaTypeImageIndex}, nil
		}
		return nil, err
	}
	var idx ociv1.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// writeIndex publishes index.json atomically (tmp + rename).
func (l *Layout) writeIndex(idx *ociv1.Index) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	// The oci-layout marker is required for a valid layout.
	if _, err := os.Stat(l.layoutFile()); os.IsNotExist(err) {
		marker, _ := json.Marshal(ociv1.ImageLayout{Version: ociv1.ImageLayoutVersion})
		if werr := writeAtomic(l.layoutFile(), marker, 0o644); werr != nil {
			return werr
		}
	}
	if idx.SchemaVersion == 0 {
		idx.SchemaVersion = 2
	}
	if idx.MediaType == "" {
		idx.MediaType = ociv1.MediaTypeImageIndex
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return writeAtomic(l.indexFile(), data, 0o644)
}

// writeAtomic writes data to path via a same-dir temp file and rename.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// findEntry locates the index descriptor whose ref.name tag equals tag.
func findEntry(idx *ociv1.Index, tag string) (ociv1.Descriptor, bool) {
	for _, d := range idx.Manifests {
		if d.Annotations[ociv1.AnnotationRefName] == tag {
			return d, true
		}
	}
	return ociv1.Descriptor{}, false
}

// --- ArtifactStore ---------------------------------------------------------

// Put stores e under key, stamping Resolved to now and publishing new blobs.
func (l *Layout) Put(key string, e Entry) error {
	e.Resolved = time.Now()
	return l.put(key, e)
}

// PutEntry stores e under key EXACTLY as given — the caller controls Resolved
// (the backdating seam for TTL tests, and the revalidating caller that refreshes
// an unchanged entry).
func (l *Layout) PutEntry(key string, e Entry) error { return l.put(key, e) }

// put is Put with the caller's Resolved preserved (the backdating seam for TTL
// tests, and the revalidating caller that refreshes an unchanged entry).
func (l *Layout) put(key string, e Entry) error {
	if !l.usable() {
		return nil
	}
	release, err := lock.AcquireFileLock(l.lockFile(), true)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	mediaType := e.MediaType
	if mediaType == "" {
		mediaType = MediaTypeCachePayload
	}
	payloadDesc, err := l.writeBlob(e.Payload, mediaType)
	if err != nil {
		return err
	}
	cfgJSON, err := json.Marshal(entryConfig{Resolved: e.Resolved, Components: e.Components, Validator: e.Validator})
	if err != nil {
		return err
	}
	cfgDesc, err := l.writeBlob(cfgJSON, MediaTypeCacheConfig)
	if err != nil {
		return err
	}
	tag := tagFor(key)
	manifest := ociv1.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ociv1.MediaTypeImageManifest,
		Config:      cfgDesc,
		Layers:      []ociv1.Descriptor{payloadDesc},
		Annotations: map[string]string{AnnotationCacheKey: key, ociv1.AnnotationRefName: tag},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDesc, err := l.writeBlob(manifestJSON, ociv1.MediaTypeImageManifest)
	if err != nil {
		return err
	}
	manifestDesc.Annotations = map[string]string{AnnotationCacheKey: key, ociv1.AnnotationRefName: tag}
	if err := l.upsertIndex(tag, manifestDesc); err != nil {
		return err
	}
	return l.enforceCap()
}

// upsertIndex replaces the descriptor tagged tag (or appends it) and publishes.
func (l *Layout) upsertIndex(tag string, desc ociv1.Descriptor) error {
	idx, err := l.readIndex()
	if err != nil {
		return err
	}
	replaced := false
	for i, d := range idx.Manifests {
		if d.Annotations[ociv1.AnnotationRefName] == tag {
			idx.Manifests[i] = desc
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Manifests = append(idx.Manifests, desc)
	}
	return l.writeIndex(idx)
}

// Get returns the entry for key and whether it was present (and well-formed).
func (l *Layout) Get(key string) (Entry, bool) {
	if !l.usable() {
		return Entry{}, false
	}
	idx, err := l.readIndex()
	if err != nil {
		return Entry{}, false
	}
	desc, ok := findEntry(idx, tagFor(key))
	if !ok {
		return Entry{}, false
	}
	manifestBytes, err := l.readBlob(desc.Digest)
	if err != nil {
		return Entry{}, false
	}
	var manifest ociv1.Manifest
	if json.Unmarshal(manifestBytes, &manifest) != nil {
		return Entry{}, false
	}
	cfgBytes, err := l.readBlob(manifest.Config.Digest)
	if err != nil {
		return Entry{}, false
	}
	var cfg entryConfig
	if json.Unmarshal(cfgBytes, &cfg) != nil {
		return Entry{}, false
	}
	var payload []byte
	mediaType := MediaTypeCachePayload
	if len(manifest.Layers) > 0 {
		payload, err = l.readBlob(manifest.Layers[0].Digest)
		if err != nil {
			return Entry{}, false
		}
		mediaType = manifest.Layers[0].MediaType
	}
	return Entry{
		Key:        key,
		Payload:    payload,
		MediaType:  mediaType,
		Resolved:   cfg.Resolved,
		Components: cfg.Components,
		Validator:  cfg.Validator,
	}, true
}

// Delete removes key's entry (a no-op when absent).
func (l *Layout) Delete(key string) error {
	if !l.usable() {
		return nil
	}
	release, err := lock.AcquireFileLock(l.lockFile(), true)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	idx, err := l.readIndex()
	if err != nil {
		return err
	}
	tag := tagFor(key)
	kept := idx.Manifests[:0]
	for _, d := range idx.Manifests {
		if d.Annotations[ociv1.AnnotationRefName] != tag {
			kept = append(kept, d)
		}
	}
	idx.Manifests = kept
	return l.writeIndex(idx)
}

// Keys returns every stored key (read from the manifest annotations).
func (l *Layout) Keys() []string {
	if !l.usable() {
		return nil
	}
	idx, err := l.readIndex()
	if err != nil {
		return nil
	}
	var keys []string
	for _, d := range idx.Manifests {
		if k := d.Annotations[AnnotationCacheKey]; k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// Len reports the entry count.
func (l *Layout) Len() int {
	if !l.usable() {
		return 0
	}
	idx, err := l.readIndex()
	if err != nil {
		return 0
	}
	return len(idx.Manifests)
}

// Clear drops the whole layout (`--invalidate` / `charly cache clear`).
func (l *Layout) Clear() error {
	if !l.usable() {
		return nil
	}
	return os.RemoveAll(l.dir)
}

// Fill returns the entry for key, computing it with fill when absent. It
// serializes the miss under the per-key flock and DOUBLE-CHECKS under the lock,
// so concurrent first-missers compute once and reuse the winner's entry. fill
// takes no argument: a caller that must REVALIDATE a mutable upstream reads the
// existing entry with Get, decides upstream itself, and Puts the refreshed
// entry.
//
// Fill STAMPS Resolved to now, exactly like Put: the compute-once path is a
// fresh computation, so its write time is now — a fill callback need not (and
// must not have to) set Resolved itself. A callback returning a zero Resolved
// (e.g. an empty degrade entry) would otherwise be instantly TTL-stale and sort
// as the OLDEST entry for reclamation.
func (l *Layout) Fill(key string, fill func() (Entry, error)) (Entry, error) {
	if !l.usable() {
		return fill()
	}
	if e, ok := l.Get(key); ok {
		return e, nil
	}
	release, err := lock.AcquireFileLock(l.fillLockPath(key), true)
	if err != nil {
		return fill()
	}
	defer func() { _ = release() }()
	if e, ok := l.Get(key); ok {
		return e, nil
	}
	e, ferr := fill()
	if ferr != nil {
		return Entry{}, ferr
	}
	e.Resolved = time.Now()
	if perr := l.put(key, e); perr != nil {
		return e, nil // the value is valid; caching is best-effort
	}
	got, ok := l.Get(key)
	if !ok {
		return e, nil
	}
	return got, nil
}

// fillLockPath is the per-key advisory-lock path for Fill's compute-once guard —
// a locks/ sibling keeps the entries dir sweepable (the same layout the legacy
// per-key keyed store used).
func (l *Layout) fillLockPath(key string) string {
	return filepath.Join(l.dir, "locks", tagFor(key)+".fill.lock")
}

// GCReclaim reports the outcome of one GC: the number of unreferenced blobs
// reclaimed and their summed size in bytes. dryRun counts the would-remove set
// without touching disk.
type GCReclaim struct {
	RemovedBlobs int
	RemovedBytes int64
}

// GC reclaims blobs no live manifest references, then enforces the entry cap.
func (l *Layout) GC() error {
	_, err := l.GCStats(false)
	return err
}

// GCStats runs GC and reports what it reclaimed. With dryRun it computes the
// same set but removes nothing — the `--dry-run` probe. A missing/inert store
// is a zero reclaim, never an error.
func (l *Layout) GCStats(dryRun bool) (GCReclaim, error) {
	if !l.usable() {
		return GCReclaim{}, nil
	}
	release, err := lock.AcquireFileLock(l.lockFile(), true)
	if err != nil {
		return GCReclaim{}, err
	}
	defer func() { _ = release() }()
	if !dryRun {
		if err := l.enforceCap(); err != nil {
			return GCReclaim{}, err
		}
	}
	idx, err := l.readIndex()
	if err != nil {
		return GCReclaim{}, err
	}
	live := map[digest.Digest]bool{}
	for _, d := range idx.Manifests {
		live[d.Digest] = true
		manifestBytes, rerr := l.readBlob(d.Digest)
		if rerr != nil {
			continue
		}
		var manifest ociv1.Manifest
		if json.Unmarshal(manifestBytes, &manifest) != nil {
			continue
		}
		live[manifest.Config.Digest] = true
		for _, layer := range manifest.Layers {
			live[layer.Digest] = true
		}
	}
	blobsRoot := filepath.Join(l.dir, ociv1.ImageBlobsDir)
	var reclaim GCReclaim
	walkErr := filepath.WalkDir(blobsRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(blobsRoot, path)
		if rerr != nil {
			return nil
		}
		alg := filepath.Base(filepath.Dir(rel))
		encoded := filepath.Base(rel)
		dgst := digest.Digest(alg + ":" + encoded)
		if live[dgst] {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			reclaim.RemovedBytes += info.Size()
		}
		reclaim.RemovedBlobs++
		if !dryRun {
			_ = os.Remove(path)
		}
		return nil
	})
	return reclaim, walkErr
}

// enforceCap reclaims the OLDEST entries when the count exceeds maxEntries.
func (l *Layout) enforceCap() error {
	if l.maxEntries <= 0 {
		return nil
	}
	idx, err := l.readIndex()
	if err != nil {
		return err
	}
	if len(idx.Manifests) <= l.maxEntries {
		return nil
	}
	type dated struct {
		desc ociv1.Descriptor
		res  time.Time
	}
	entries := make([]dated, 0, len(idx.Manifests))
	for _, d := range idx.Manifests {
		res := time.Time{}
		if mb, rerr := l.readBlob(d.Digest); rerr == nil {
			var m ociv1.Manifest
			if json.Unmarshal(mb, &m) == nil {
				if cb, cerr := l.readBlob(m.Config.Digest); cerr == nil {
					var cfg entryConfig
					if json.Unmarshal(cb, &cfg) == nil {
						res = cfg.Resolved
					}
				}
			}
		}
		entries = append(entries, dated{d, res})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].res.Before(entries[j].res) })
	excess := len(entries) - l.maxEntries
	drop := map[string]bool{}
	for i := 0; i < excess; i++ {
		drop[entries[i].desc.Annotations[ociv1.AnnotationRefName]] = true
	}
	kept := idx.Manifests[:0]
	for _, d := range idx.Manifests {
		if !drop[d.Annotations[ociv1.AnnotationRefName]] {
			kept = append(kept, d)
		}
	}
	idx.Manifests = kept
	return l.writeIndex(idx)
}

// HashHex is the hex SHA-256 of s — the content-addressed digest helper shared
// by every components-keyed caller.
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
