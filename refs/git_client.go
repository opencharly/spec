package refs

// git_client.go — the centralized GIT LAYER: the single entry point every charly
// command, plugin, and loader uses for git operations. It wraps the raw git
// primitives (GitLatestTag / GitDefaultBranch / GitResolveRef / GitClone) with a
// persistent, project-scoped cache, so a git command runs ONLY when the answer is
// not already known — never once per call site per resolution.
//
// Why this exists: before this layer, every consumer called the raw primitives
// directly (loaderkit's refs_collect/canonical_ref/scan_orchestrate, charly core's
// host_build_* seams), and the project load resolved EVERY version-less @github ref
// with a fresh `git ls-remote` — 30+ network calls on the charly repo's own project,
// paid by every command including `charly version` (issue #423, #208).
//
// The cache is PROJECT-SCOPED: it lives under the project's charly directory
// (CHARLY_PROJECT_DIR/.charly/cache/git.json, or the repo cache dir when no project
// is present), so each project's resolved refs persist across invocations and are
// shared by every command in that project.
//
// Freshness policy (the load-bearing contract):
//   - latest-tag: tags are IMMUTABLE and add-only → a cached value is valid until a
//     newer tag appears; a long TTL (24h) is safe, and the next expiry refreshes.
//   - default-branch: the branch NAME is stable → a TTL (24h) is safe.
//   - resolve-ref (the DownloadRepo freshness check): a mutable branch can move, so
//     this is cached with a SHORT TTL (5m) — the freshness contract requires seeing
//     the live commit of a mutable branch, but not on every single invocation.
//
// The cache is guarded by the same advisory file-lock primitive the repo fetch uses,
// so concurrent resolvers serialize their read-modify-write and cannot lose updates.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/opencharly/spec/lock"
)

// Cache TTLs (see the freshness policy above).
const (
	LatestTagTTL     = 24 * time.Hour
	DefaultBranchTTL = 24 * time.Hour
	ResolveRefTTL    = 5 * time.Minute
)

// GitClient is the centralized git layer. Construct once per process (or per
// project) and share it — the cache is the point.
type GitClient struct {
	cacheDir string // the project's charly dir cache (or the repo cache dir)

	mu              sync.Mutex
	latestTags      map[string]gitCacheEntry
	defaultBranches map[string]gitCacheEntry
	resolvedRefs    map[string]gitCacheEntry
	downloads       map[string]gitCacheEntry
}

type gitCacheEntry struct {
	Value    string    `json:"value"`
	Resolved time.Time `json:"resolved"`
}

// NewGitClient returns a GitClient whose cache lives under cacheDir. When cacheDir
// is empty, the repo cache dir (~/.cache/charly/repos) is used.
func NewGitClient(cacheDir string) *GitClient {
	if cacheDir == "" {
		cacheDir, _ = RepoCacheDir()
	}
	g := &GitClient{
		cacheDir:        cacheDir,
		latestTags:      map[string]gitCacheEntry{},
		defaultBranches: map[string]gitCacheEntry{},
		resolvedRefs:    map[string]gitCacheEntry{},
		downloads:       map[string]gitCacheEntry{},
	}
	g.load() // read the persisted cache so a warm cache is honored across invocations
	return g
}

// cacheFile is the JSON cache path under the cache dir.
func (g *GitClient) cacheFile() string {
	return filepath.Join(g.cacheDir, "git-cache.json")
}

// load reads the persisted cache (best-effort; a corrupt/absent file starts empty).
func (g *GitClient) load() {
	data, err := os.ReadFile(g.cacheFile())
	if err != nil {
		return
	}
	var persisted struct {
		LatestTags      map[string]gitCacheEntry `json:"latest_tags"`
		DefaultBranches map[string]gitCacheEntry `json:"default_branches"`
		ResolvedRefs    map[string]gitCacheEntry `json:"resolved_refs"`
		Downloads       map[string]gitCacheEntry `json:"downloads"`
	}
	if json.Unmarshal(data, &persisted) != nil {
		return
	}
	if persisted.LatestTags != nil {
		g.latestTags = persisted.LatestTags
	}
	if persisted.DefaultBranches != nil {
		g.defaultBranches = persisted.DefaultBranches
	}
	if persisted.ResolvedRefs != nil {
		g.resolvedRefs = persisted.ResolvedRefs
	}
	if persisted.Downloads != nil {
		g.downloads = persisted.Downloads
	}
}

// save persists the cache under the advisory lock (best-effort).
func (g *GitClient) save() {
	lockPath := g.cacheFile() + ".lock"
	unlock, err := lock.AcquireFileLock(lockPath, true)
	if err != nil {
		return
	}
	defer unlock()
	if err := os.MkdirAll(g.cacheDir, 0o755); err != nil {
		return
	}
	persisted := struct {
		LatestTags      map[string]gitCacheEntry `json:"latest_tags"`
		DefaultBranches map[string]gitCacheEntry `json:"default_branches"`
		ResolvedRefs    map[string]gitCacheEntry `json:"resolved_refs"`
		Downloads       map[string]gitCacheEntry `json:"downloads"`
	}{g.latestTags, g.defaultBranches, g.resolvedRefs, g.downloads}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(g.cacheFile(), data, 0o644)
}

// cached returns the cached value for key if fresh, or "".
func cached(entries map[string]gitCacheEntry, key string, ttl time.Duration) string {
	e, ok := entries[key]
	if !ok {
		return ""
	}
	if time.Since(e.Resolved) > ttl {
		return ""
	}
	return e.Value
}

// LatestTag returns the highest semver tag of repoURL, cached (tags are immutable).
func (g *GitClient) LatestTag(repoURL string) (string, error) {
	g.mu.Lock()
	if v := cached(g.latestTags, repoURL, LatestTagTTL); v != "" {
		g.mu.Unlock()
		return v, nil
	}
	g.mu.Unlock()

	tag, err := GitLatestTag(repoURL)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	g.latestTags[repoURL] = gitCacheEntry{Value: tag, Resolved: time.Now()}
	g.save()
	g.mu.Unlock()
	return tag, nil
}

// DefaultBranch returns the default branch of repoURL, cached (the name is stable).
func (g *GitClient) DefaultBranch(repoURL string) (string, error) {
	g.mu.Lock()
	if v := cached(g.defaultBranches, repoURL, DefaultBranchTTL); v != "" {
		g.mu.Unlock()
		return v, nil
	}
	g.mu.Unlock()

	branch, err := GitDefaultBranch(repoURL)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	g.defaultBranches[repoURL] = gitCacheEntry{Value: branch, Resolved: time.Now()}
	g.save()
	g.mu.Unlock()
	return branch, nil
}

// ResolveRef resolves a ref to a commit SHA, cached with a SHORT TTL (the freshness
// contract: a mutable branch can move, but not on every invocation).
func (g *GitClient) ResolveRef(repoURL, ref string) (string, error) {
	key := repoURL + " " + ref
	g.mu.Lock()
	if v := cached(g.resolvedRefs, key, ResolveRefTTL); v != "" {
		g.mu.Unlock()
		return v, nil
	}
	g.mu.Unlock()

	sha, err := GitResolveRef(repoURL, ref)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	g.resolvedRefs[key] = gitCacheEntry{Value: sha, Resolved: time.Now()}
	g.save()
	g.mu.Unlock()
	return sha, nil
}

// WarmUp prefetches the latest tag and default branch for a set of repo URLs,
// printing a progress line to stderr on the first (cold) run so the user knows
// charly is fetching git metadata. It is the "first startup" feedback the CLI
// shows before a command that will need the refs.
func (g *GitClient) WarmUp(repoURLs []string, stderr *os.File) {
	var cold []string
	for _, u := range repoURLs {
		g.mu.Lock()
		have := cached(g.latestTags, u, LatestTagTTL) != "" && cached(g.defaultBranches, u, DefaultBranchTTL) != ""
		g.mu.Unlock()
		if !have {
			cold = append(cold, u)
		}
	}
	if len(cold) == 0 {
		return
	}
	fmt.Fprintf(stderr, "charly: fetching git metadata for %d repo(s) (first run — may take a moment)...\n", len(cold))
	for _, u := range cold {
		_, _ = g.LatestTag(u)
		_, _ = g.DefaultBranch(u)
	}
	fmt.Fprintf(stderr, "charly: git metadata cached.\n")
}

// Download fetches repoPath@version into the repo cache and returns the cache path,
// CACHED with a short TTL. A mutable ref (a branch or the default branch) can move,
// so the freshness contract requires re-resolving eventually — but not on every
// invocation. The 5m TTL means a command run twice in quick succession (e.g. the
// status fan-out resolving the envelope multiple times) pays the download once.
func (g *GitClient) Download(repoPath, version string, download func(repoPath, version string) (string, error)) (string, error) {
	key := repoPath + "@" + version
	g.mu.Lock()
	if v := cached(g.downloads, key, ResolveRefTTL); v != "" {
		g.mu.Unlock()
		return v, nil
	}
	g.mu.Unlock()

	path, err := download(repoPath, version)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	g.downloads[key] = gitCacheEntry{Value: path, Resolved: time.Now()}
	g.save()
	g.mu.Unlock()
	return path, nil
}
