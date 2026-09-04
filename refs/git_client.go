package refs

// git_client.go — the centralized GIT LAYER: the single entry point every charly
// command, plugin, and loader uses for git operations. It wraps the raw git
// primitives (GitLatestTag / GitDefaultBranch / GitResolveRef / GitClone) with a
// persistent cache, so a git command runs ONLY when the answer is not already
// known — never once per call site per resolution.
//
// Why this exists: before this layer, every consumer called the raw primitives
// directly (loaderkit's refs_collect/canonical_ref/scan_orchestrate, charly core's
// host_build_* seams), and the project load resolved EVERY version-less @github ref
// with a fresh `git ls-remote` — 30+ network calls on the charly repo's own project,
// paid by every command including `charly version` (issue #423, #208).
//
// The cache lives in the `cache:` section of the PER-HOST charly.yml
// (~/.config/charly/charly.yml) — the single home for local system state
// (deployments under `deploy:`, install records under `ledger:`, local system
// info under `system:`, cache status under `cache:`). It is NOT a separate ad-hoc
// JSON file under ~/.cache/charly/repos (git-cache.json / latest-tags.json — both
// deleted by this cutover). The `cache:` shape is CUE-sourced (schema/cache.cue
// #CacheConfig) and validated whenever the per-host file is loaded through the
// unified loader; the GitClient itself does a lightweight YAML read-modify-write
// of just the `cache:` key (preserving every other key), under the same advisory
// file-lock primitive the repo fetch uses.
//
// Freshness policy (the load-bearing contract):
//   - latest-tag: tags are IMMUTABLE and add-only → a cached value is valid until a
//     newer tag appears; a long TTL (24h) is safe, and the next expiry refreshes.
//   - default-branch: the branch NAME is stable → a TTL (24h) is safe.
//   - resolve-ref (the DownloadRepo freshness check): a mutable branch can move, so
//     this is cached with a SHORT TTL (5m) — the freshness contract requires seeing
//     the live commit of a mutable branch, but not on every single invocation.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/opencharly/spec/lock"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// Cache TTLs (see the freshness policy above).
const (
	LatestTagTTL     = 24 * time.Hour
	DefaultBranchTTL = 24 * time.Hour
	// ResolveRefTTL keeps resolved SHAs for one hour. The eval-batch reality: a 16-lane,
	// multi-phase check run re-resolves the same refs hundreds of times; a 5-minute TTL
	// re-probes mid-batch (measured: 152 concurrent git ls-remote -> GitHub throttling ->
	// the deploy-add stall). Branches move at most hourly; plan-pinned eval heads never move.
	ResolveRefTTL = time.Hour
)

// GitClient is the centralized git layer. Construct once per process (or per
// project) and share it — the cache is the point.
type GitClient struct {
	cacheFile string // the per-host charly.yml path holding the `cache:` section

	mu              sync.Mutex
	latestTags      map[string]gitCacheEntry
	defaultBranches map[string]gitCacheEntry
	resolvedRefs    map[string]gitCacheEntry
	downloads       map[string]gitCacheEntry
}

type gitCacheEntry struct {
	Value    string    `yaml:"value"`
	Resolved time.Time `yaml:"resolved"`
}

// NewGitClient returns a GitClient whose cache lives in the `cache:` section of
// the per-host charly.yml at cacheFile. When cacheFile is empty, the default
// deploy config path (~/.config/charly/charly.yml, honoring the
// CHARLY_DEPLOY_CONFIG override) is used.
func NewGitClient(cacheFile string) *GitClient {
	if cacheFile == "" {
		cacheFile, _ = spec.DefaultDeployConfigPath()
	}
	g := &GitClient{
		cacheFile:       cacheFile,
		latestTags:      map[string]gitCacheEntry{},
		defaultBranches: map[string]gitCacheEntry{},
		resolvedRefs:    map[string]gitCacheEntry{},
		downloads:       map[string]gitCacheEntry{},
	}
	g.load() // read the persisted cache so a warm cache is honored across invocations
	return g
}

// cacheLockPath is the advisory lock path guarding the cache's read-modify-write.
// A separate .lock file keeps the lock from clobbering the config's own bytes.
func (g *GitClient) cacheLockPath() string {
	return g.cacheFile + ".lock"
}

// load reads the `cache:` section of the per-host charly.yml (best-effort; a
// corrupt/absent file starts empty).
func (g *GitClient) load() {
	data, err := os.ReadFile(g.cacheFile)
	if err != nil {
		return
	}
	var doc struct {
		Cache *struct {
			Git *struct {
				LatestTags      map[string]gitCacheEntry `yaml:"latest_tags"`
				DefaultBranches map[string]gitCacheEntry `yaml:"default_branches"`
				ResolvedRefs    map[string]gitCacheEntry `yaml:"resolved_refs"`
				Downloads       map[string]gitCacheEntry `yaml:"downloads"`
			} `yaml:"git"`
		} `yaml:"cache"`
	}
	if yaml.Unmarshal(data, &doc) != nil {
		return
	}
	if doc.Cache == nil || doc.Cache.Git == nil {
		return
	}
	if doc.Cache.Git.LatestTags != nil {
		g.latestTags = doc.Cache.Git.LatestTags
	}
	if doc.Cache.Git.DefaultBranches != nil {
		g.defaultBranches = doc.Cache.Git.DefaultBranches
	}
	if doc.Cache.Git.ResolvedRefs != nil {
		g.resolvedRefs = doc.Cache.Git.ResolvedRefs
	}
	if doc.Cache.Git.Downloads != nil {
		g.downloads = doc.Cache.Git.Downloads
	}
}

// save persists the `cache:` section into the per-host charly.yml under the
// advisory lock (best-effort). It reads the CURRENT file, updates only the
// `cache:` key (preserving every other key — deploy:, provides:, ledger:,
// system:, …), and writes back atomically (tempfile + rename).
func (g *GitClient) save() {
	unlock, err := lock.AcquireFileLock(g.cacheLockPath(), true)
	if err != nil {
		return
	}
	defer func() { _ = unlock() }()

	// Read the current file (may not exist yet — a fresh host starts empty).
	data, err := os.ReadFile(g.cacheFile)
	var doc yaml.Node
	if err == nil {
		if yaml.Unmarshal(data, &doc) != nil {
			return // corrupt file — never clobber it
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Tag: "!!map"},
		}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return
	}

	// Ensure the HEAD schema version stamp is present — the per-host charly.yml
	// is loaded through the unified loader, which requires the version directive.
	// A fresh file created by the cache write must carry it, or the loader rejects
	// the file ("schema X is required (found \"\")").
	if !hasMappingKey(root, "version") {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "version"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: spec.SchemaVersion},
		)
	}

	// Find or create the `cache` key.
	var cacheVal *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "cache" {
			cacheVal = root.Content[i+1]
			break
		}
	}
	if cacheVal == nil {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "cache"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
		cacheVal = root.Content[len(root.Content)-1]
	}

	// Build the cache: git: {latest_tags, default_branches, resolved_refs, downloads}.
	gitVal := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "latest_tags"},
		entryMapNode(g.latestTags),
		{Kind: yaml.ScalarNode, Value: "default_branches"},
		entryMapNode(g.defaultBranches),
		{Kind: yaml.ScalarNode, Value: "resolved_refs"},
		entryMapNode(g.resolvedRefs),
		{Kind: yaml.ScalarNode, Value: "downloads"},
		entryMapNode(g.downloads),
	}}
	cacheVal.Kind = yaml.MappingNode
	cacheVal.Tag = "!!map"
	cacheVal.Content = []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "git"},
		gitVal,
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return
	}
	// Atomic write: tempfile in the same dir + rename.
	dir := filepath.Dir(g.cacheFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".charly-cache-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, g.cacheFile); err != nil {
		_ = os.Remove(tmpName)
	}
}

// hasMappingKey reports whether a mapping node has a top-level key with the given
// name.
func hasMappingKey(m *yaml.Node, name string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == name {
			return true
		}
	}
	return false
}

// entryMapNode builds a YAML mapping node from a cache-entry map.
func entryMapNode(entries map[string]gitCacheEntry) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	// Deterministic output: sort keys so the file is stable across runs.
	sort.Strings(keys)
	for _, k := range keys {
		e := entries[k]
		n.Content = append(n.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "value"},
				{Kind: yaml.ScalarNode, Value: e.Value},
				{Kind: yaml.ScalarNode, Value: "resolved"},
				{Kind: yaml.ScalarNode, Value: e.Resolved.UTC().Format(time.RFC3339)},
			}},
		)
	}
	return n
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
	// Parallelize the fetch with a bounded worker pool: each repo is an
	// independent `git ls-remote` (network-bound), so a sequential loop pays the
	// round-trip latency once per repo — 200 repos × ~1.5s ≈ 5 minutes on a cold
	// cache. 10 workers collapse that to ~30s. The GitClient methods are
	// mutex-guarded, so concurrent warm-up is safe; the advisory file lock
	// serializes the cache writes.
	const warmUpWorkers = 10
	jobs := make(chan string)
	var wg sync.WaitGroup
	for range warmUpWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				_, _ = g.LatestTag(u)
				_, _ = g.DefaultBranch(u)
			}
		}()
	}
	for _, u := range cold {
		jobs <- u
	}
	close(jobs)
	wg.Wait()
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
