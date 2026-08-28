package refs

// latest_tag_cache.go — the persistent latest-tag cache for GitLatestTag.
//
// GitLatestTag runs `git ls-remote --tags <repo>` — a NETWORK operation — for every
// version-less @github ref the resolver walks. A `charly status` on a project whose
// closure carries hundreds of version-less refs (the distro submodules + the standalone
// candy repos) paid for that network round-trip once per ref PER RESOLUTION, and the
// status fan-out resolves the envelope multiple times — measured at 366 git-remote-https
// invocations and ~140s of futex wait in one `charly status` (issue #208).
//
// Tags are IMMUTABLE and add-only, so a cached latest tag is valid until a NEWER tag
// appears. A TTL-bounded cache (default 1h) is therefore safe: a stale entry is at worst
// one tag behind, and the next expiry refreshes it. The cache lives beside the repo cache
// (~/.cache/charly/repos/latest-tags.json) and every WRITE is guarded by the same
// advisory file-lock primitive the repo fetch uses, so concurrent resolvers serialize
// their read-modify-write and cannot lose updates or corrupt the file.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/opencharly/spec/lock"
)

// latestTagTTL is how long a cached latest-tag entry is trusted before a network refresh.
const latestTagTTL = time.Hour

// latestTagCacheFile is the JSON cache path (under the repo cache dir).
func latestTagCacheFile() (string, error) {
	dir, err := RepoCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "latest-tags.json"), nil
}

// latestTagCacheLock is the advisory lock path guarding the cache's read-modify-write.
// A separate .lock file keeps the lock from clobbering the cache's own bytes.
func latestTagCacheLock() (string, error) {
	file, err := latestTagCacheFile()
	if err != nil {
		return "", err
	}
	return file + ".lock", nil
}

type latestTagEntry struct {
	Tag      string    `json:"tag"`
	Resolved time.Time `json:"resolved"`
}

// defaultBranchEntry is the cached default-branch value for a repo URL.
type defaultBranchEntry struct {
	Branch   string    `json:"branch"`
	Resolved time.Time `json:"resolved"`
}

// resolvedRefEntry is the cached GitResolveRef result (repo URL + ref → commit).
type resolvedRefEntry struct {
	Commit   string    `json:"commit"`
	Resolved time.Time `json:"resolved"`
}

type latestTagCache struct {
	Entries       map[string]latestTagEntry     `json:"entries"`
	DefaultBranch map[string]defaultBranchEntry `json:"default_branches,omitempty"`
	ResolvedRefs  map[string]resolvedRefEntry   `json:"resolved_refs,omitempty"`
}

func loadLatestTagCache() (latestTagCache, error) {
	var c latestTagCache
	path, err := latestTagCacheFile()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.Entries = map[string]latestTagEntry{}
			c.DefaultBranch = map[string]defaultBranchEntry{}
			c.ResolvedRefs = map[string]resolvedRefEntry{}
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		// A corrupt cache is not fatal — start fresh.
		c.Entries = map[string]latestTagEntry{}
		c.DefaultBranch = map[string]defaultBranchEntry{}
		c.ResolvedRefs = map[string]resolvedRefEntry{}
		return c, nil
	}
	if c.Entries == nil {
		c.Entries = map[string]latestTagEntry{}
	}
	if c.DefaultBranch == nil {
		c.DefaultBranch = map[string]defaultBranchEntry{}
	}
	if c.ResolvedRefs == nil {
		c.ResolvedRefs = map[string]resolvedRefEntry{}
	}
	return c, nil
}

func saveLatestTagCache(c latestTagCache) error {
	path, err := latestTagCacheFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// withLatestTagCacheLock serializes a read-modify-write of the cache file. The advisory
// lock is held for the whole load → mutate → save cycle, so two concurrent resolvers
// cannot lose each other's entries or interleave writes into a corrupt file.
func withLatestTagCacheLock(mutate func(c *latestTagCache) error) error {
	lockPath, err := latestTagCacheLock()
	if err != nil {
		return err
	}
	release, err := lock.AcquireFileLock(lockPath, true)
	if err != nil {
		return err
	}
	defer release()

	c, err := loadLatestTagCache()
	if err != nil {
		return err
	}
	if err := mutate(&c); err != nil {
		return err
	}
	return saveLatestTagCache(c)
}

// cachedLatestTag returns the cached latest tag for repoURL if it is fresh, or "".
// Reads are lock-free: a stale read is harmless (the writer serializes), and the
// fast path must not pay for a lock on every status invocation.
func cachedLatestTag(repoURL string) string {
	c, err := loadLatestTagCache()
	if err != nil {
		return ""
	}
	e, ok := c.Entries[repoURL]
	if !ok {
		return ""
	}
	if time.Since(e.Resolved) > latestTagTTL {
		return ""
	}
	return e.Tag
}

// rememberLatestTag stores repoURL's latest tag in the persistent cache.
func rememberLatestTag(repoURL, tag string) {
	_ = withLatestTagCacheLock(func(c *latestTagCache) error {
		c.Entries[repoURL] = latestTagEntry{Tag: tag, Resolved: time.Now()}
		return nil
	})
}

// cachedDefaultBranch returns the cached default branch for repoURL if fresh, or "".
func cachedDefaultBranch(repoURL string) string {
	c, err := loadLatestTagCache()
	if err != nil {
		return ""
	}
	e, ok := c.DefaultBranch[repoURL]
	if !ok {
		return ""
	}
	if time.Since(e.Resolved) > latestTagTTL {
		return ""
	}
	return e.Branch
}

// rememberDefaultBranch stores repoURL's default branch in the persistent cache.
func rememberDefaultBranch(repoURL, branch string) {
	_ = withLatestTagCacheLock(func(c *latestTagCache) error {
		c.DefaultBranch[repoURL] = defaultBranchEntry{Branch: branch, Resolved: time.Now()}
		return nil
	})
}

// resolvedRefCacheKey is the map key for a GitResolveRef result.
func resolvedRefCacheKey(repoURL, ref string) string {
	return repoURL + " " + ref
}

// cachedResolvedRef returns the cached GitResolveRef commit for repoURL+ref if fresh.
func cachedResolvedRef(repoURL, ref string) string {
	c, err := loadLatestTagCache()
	if err != nil {
		return ""
	}
	e, ok := c.ResolvedRefs[resolvedRefCacheKey(repoURL, ref)]
	if !ok {
		return ""
	}
	if time.Since(e.Resolved) > latestTagTTL {
		return ""
	}
	return e.Commit
}

// rememberResolvedRef stores a GitResolveRef result in the persistent cache.
func rememberResolvedRef(repoURL, ref, commit string) {
	_ = withLatestTagCacheLock(func(c *latestTagCache) error {
		c.ResolvedRefs[resolvedRefCacheKey(repoURL, ref)] = resolvedRefEntry{Commit: commit, Resolved: time.Now()}
		return nil
	})
}
