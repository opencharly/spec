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
// Freshness policy (the load-bearing contract — the Docker model, NO TTL):
//   - every remote answer (latest-tag, default-branch, resolve-ref, download-path)
//     is a function of REMOTE state that can move with no local content change, so
//     it is NOT content-addressable locally and is NEVER reused across processes.
//     The in-process maps are LIFETIME MEMOS: they dedupe the within-process
//     re-resolutions a single command/wave performs, and a fresh process re-probes
//     on demand (this is what `docker pull` does — it does not serve a 1h-old
//     remote digest).
//   - latest-tag: the persisted entry is an OFFLINE FALLBACK served ONLY when the
//     network fetch fails, with a loud stderr line — never fresh data (the former
//     1h persisted-TTL reuse was the stale-latest-tag bug).
//   - download-path: the cached path is served only while its CONTENT is present
//     (dirUsable) — the content-validity half.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/opencharly/spec/lock"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// NO TTL. Remote ref resolution is NOT content-addressable locally (a branch or a
// tag on the remote can move with no local content change to hash), so — exactly
// like `docker pull` — there is no cross-process reuse of a remote answer. The
// in-process maps are LIFETIME MEMOS: they dedupe the many re-resolutions a single
// command/wave performs (the measured 152-concurrent-ls-remote throttling storm was
// a WITHIN-one-process fanout), and a fresh process re-probes on demand. The
// persisted latest_tags entry is an OFFLINE FALLBACK only (served when the fetch
// fails, with a loud stderr line) — never a fresh answer.
//
// The local repo-cache DIR is the content-addressed half: Download serves a cached
// path only while dirUsable (the fetched content is present), the same
// content-validity rule the image/label caches use.

// GitClient is the centralized git layer. Construct once per process (or per
// project) and share it — the cache is the point.
type GitClient struct {
	cacheFile string // the per-host charly.yml path holding the `cache:` section
	disabled  bool   // BypassCache()/SetBypass — every lookup misses, so the next resolution is fresh
	bypass    bool   // the PERSISTED bypass flag (SetBypass) — honored by a fresh client at construction

	mu sync.Mutex
	// latestTags is the IN-PROCESS fresh-data cache: only entries this process
	// fetched live here (the batch-dedupe layer). Persisted entries from disk
	// NEVER land here — see persistedTags.
	latestTags map[string]gitCacheEntry
	// persistedTags is the OFFLINE FALLBACK loaded from the per-host charly.yml at
	// construction: resilience data, never fresh data. It is served ONLY when the
	// network fetch fails (with a loud stderr line) and is preserved on save (the
	// on-disk latest_tags map is the union of persistedTags and latestTags, fresh
	// values winning). Serving persisted entries as fresh was the stale-latest-tag
	// bug: a fresh process minutes after a tag push served the still-cached old tag.
	persistedTags   map[string]gitCacheEntry
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
		persistedTags:   map[string]gitCacheEntry{},
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
				Bypass          bool                     `yaml:"bypass"`
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
	// The persisted latest_tags entries are the OFFLINE FALLBACK (never fresh
	// data) — they load into persistedTags, never into the in-process fresh cache.
	if doc.Cache.Git.LatestTags != nil {
		g.persistedTags = doc.Cache.Git.LatestTags
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
	if doc.Cache.Git.Bypass {
		g.disabled = true
		g.bypass = true
	}
}

// save persists the `cache:` section into the per-host charly.yml under the
// advisory lock (best-effort). It reads the CURRENT file, updates only the
// `cache:` key (preserving every other key — deploy:, provides:, ledger:,
// system:, …), and writes back atomically (tempfile + rename).
// BypassCache disables every cached lookup — the next resolutions are fresh
// (an operator on-demand truth: a new tag just released, a moved branch).
// Runtime-only: the flag does not survive the process (see SetBypass for the
// persisted form).
func (g *GitClient) BypassCache() {
	g.mu.Lock()
	g.disabled = true
	g.mu.Unlock()
}

// SetBypass persists the bypass flag in the cache: git: section of the per-host
// charly.yml — the `charly cache bypass` operator surface. When on, every cached
// lookup is disabled (fresh resolutions) until turned off, and a NEW process (a
// fresh GitClient) honors it at construction (load reads the flag). When off, the
// flag is cleared and the cache resumes.
func (g *GitClient) SetBypass(on bool) error {
	g.mu.Lock()
	g.disabled = on
	g.bypass = on
	g.mu.Unlock()
	g.save()
	return nil
}

// CacheStatus reports the cache file path and the number of cached git answers
// (the operator-facing `charly cache status` surface).
func (g *GitClient) CacheStatus() (string, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cacheFile, len(g.mergedLatestTags()) + len(g.defaultBranches) + len(g.resolvedRefs) + len(g.downloads)
}

// ClearCache drops every cached git answer (the in-memory entries and the persisted
// file), so the next resolutions are fresh — the `charly cache clear/refresh` surface.
func (g *GitClient) ClearCache() error {
	g.mu.Lock()
	g.latestTags = map[string]gitCacheEntry{}
	g.persistedTags = map[string]gitCacheEntry{}
	g.defaultBranches = map[string]gitCacheEntry{}
	g.resolvedRefs = map[string]gitCacheEntry{}
	g.downloads = map[string]gitCacheEntry{}
	file := g.cacheFile
	g.mu.Unlock()
	if file != "" {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

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

	// Build the cache: git: {bypass, latest_tags, default_branches, resolved_refs, downloads}.
	gitVal := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "bypass"},
		{Kind: yaml.ScalarNode, Value: strconv.FormatBool(g.bypass)},
		{Kind: yaml.ScalarNode, Value: "latest_tags"},
		entryMapNode(g.mergedLatestTags()),
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

// dirUsable reports whether a cached DOWNLOAD path still holds its materialized
// export (a directory). The downloads map serves PATHS; a wiped or evicted
// repo-cache dir must never be served — a cache result is only valid while its
// CONTENT is valid (the same content-validity rule the image/label and
// materialized-tree caches enforce). A missing or non-directory path is a miss,
// so the downloader repopulates it.
func dirUsable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// memo returns the in-process lifetime memo for key, or "". There is NO time
// validity: the entry lives for the life of the client (one process/command),
// deduping the within-process re-resolutions; a fresh process re-probes. This is
// the Docker-like model for remote mutable state — no cross-process reuse.
func memo(entries map[string]gitCacheEntry, key string) string {
	e, ok := entries[key]
	if !ok {
		return ""
	}
	return e.Value
}

// mergedLatestTags returns the persisted offline-fallback entries overlaid with the
// in-process fresh resolutions (fresh values winning) — the map save() writes, so a
// process that never fetched a repo's tags still preserves the fallback entries on
// disk (a DefaultBranch save must not wipe latest_tags offline resilience).
func (g *GitClient) mergedLatestTags() map[string]gitCacheEntry {
	merged := make(map[string]gitCacheEntry, len(g.persistedTags)+len(g.latestTags))
	for k, v := range g.persistedTags {
		merged[k] = v
	}
	for k, v := range g.latestTags {
		merged[k] = v
	}
	return merged
}

// gitLatestTagFetch is the network fetch seam LatestTag drives (package var so the
// offline-fallback tests inject fetch outcomes without shelling git).
var gitLatestTagFetch = GitLatestTag

// LatestTag returns the highest semver tag of repoURL.
//
// Freshness contract (the offline-fallback cutover): the IN-PROCESS map is the
// only fresh-data cache (a LIFETIME MEMO deduping within one process — the
// 152-concurrent-ls-remote throttling storm was a within-one-process fanout,
// solved by this layer). The PERSISTED latest_tags entries are an OFFLINE
// FALLBACK, never fresh data: a fresh process with a reachable network re-probes,
// so a tag pushed minutes ago is always seen. (Serving persisted entries as fresh
// was the stale-latest-tag bug: a fresh run resolved plugin refs to a tag list up
// to an hour old.) The persisted entry is served ONLY when the network fetch
// fails, with a loud stderr line — resilience, not freshness.
func (g *GitClient) LatestTag(repoURL string) (string, error) {
	g.mu.Lock()
	if !g.disabled {
		if v := memo(g.latestTags, repoURL); v != "" {
			g.mu.Unlock()
			return v, nil
		}
	}
	g.mu.Unlock()

	tag, err := gitLatestTagFetch(repoURL)
	if err != nil {
		g.mu.Lock()
		fallback, ok := g.persistedTags[repoURL]
		g.mu.Unlock()
		if ok && fallback.Value != "" {
			fmt.Fprintf(os.Stderr, "charly: git latest-tag fetch for %s failed (%v); serving the persisted OFFLINE-FALLBACK tag %s (resolved %s)\n", repoURL, err, fallback.Value, fallback.Resolved.UTC().Format(time.RFC3339))
			return fallback.Value, nil
		}
		return "", err
	}
	g.mu.Lock()
	g.latestTags[repoURL] = gitCacheEntry{Value: tag, Resolved: time.Now()}
	g.save()
	g.mu.Unlock()
	return tag, nil
}
func (g *GitClient) DefaultBranch(repoURL string) (string, error) {
	g.mu.Lock()
	if !g.disabled {
		if v := memo(g.defaultBranches, repoURL); v != "" {
			g.mu.Unlock()
			return v, nil
		}
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
func (g *GitClient) ResolveRef(repoURL, ref string) (string, error) {
	key := repoURL + " " + ref
	g.mu.Lock()
	if !g.disabled {
		if v := memo(g.resolvedRefs, key); v != "" {
			g.mu.Unlock()
			return v, nil
		}
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
func (g *GitClient) WarmUp(repoURLs []string, stderr *os.File) {
	var cold []string
	for _, u := range repoURLs {
		g.mu.Lock()
		// The prefetch cold-check treats a PERSISTED latest_tags entry as
		// prefetch-warm. This does NOT weaken the freshness contract: the persisted
		// entries remain the OFFLINE FALLBACK, never fresh data — every on-demand
		// LatestTag call still re-probes the network in a fresh process (the
		// offline-fallback cutover, #108). WarmUp is a UX batch, not a resolution:
		// skipping the prefetch for a repo the fallback already knows only defers
		// the probe to the on-demand path (which for a repo pinned at immutable
		// tags never fires at all). Without this, a fresh process with a fully
		// persisted cache reported EVERY repo cold and re-probed the entire corpus
		// on every project load — the observed 194-repo warm-up hang under the
		// multi-bed wave (the pre-#108 model served the persisted entry as fresh,
		// so the prefetch found one cold repo and returned instantly).
		have := (memo(g.latestTags, u) != "" || g.persistedTags[u].Value != "") &&
			memo(g.defaultBranches, u) != ""
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

// Download fetches repoPath@version into the repo cache and returns the cache path.
// The in-process map is a LIFETIME MEMO: a command that resolves the envelope several
// times (the status fan-out) pays the download once, while a fresh process re-resolves.
// The cached PATH is validated against its CONTENT (dirUsable): a wiped or evicted
// repo-cache dir is a miss, so the downloader repopulates it (the content-validity
// rule — the same class the materialized-tree cache enforces via component drift
// detection).
func (g *GitClient) Download(repoPath, version string, download func(repoPath, version string) (string, error)) (string, error) {
	key := repoPath + "@" + version
	g.mu.Lock()
	if v := memo(g.downloads, key); v != "" && dirUsable(v) {
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
