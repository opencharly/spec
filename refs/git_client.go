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
//   - latest-tag: tags are IMMUTABLE and add-only, but a NEW tag can appear at any
//     moment (this org tags releases minutes apart) — the persisted entries are an
//     OFFLINE FALLBACK served ONLY when the network fetch fails, never fresh data;
//     the in-process cache (1h) is the batch-dedupe layer. A fresh process with a
//     reachable network ALWAYS re-probes, so a fresh run never serves a stale tag
//     list (the former 1h persisted-TTL reuse was the stale-latest-tag bug).
//   - default-branch: the branch NAME is stable → a TTL (24h is safe; the shared
//     batch-dedupe default is used) is safe.
//   - download (the DownloadRepo freshness check): NOT persisted and NOT time-validated. The
//     answer to "which commit does this ref name NOW" is CONTENT that a mutable branch can move,
//     so it is re-taken from upstream (`GitResolveRef`) per process, deduped in-process for the
//     batch. A resolved ref's DURABLE record is the export's own v2 provenance sidecar
//     (RepoCacheProvenance), which names the commit it was cloned from — its content identity.
//
// CONTENT, NOT PRESENCE. No entry in this file is served because a path exists. Every cached
// export is certified on the read path (refs.IsCertifiedExport: directory + v2 provenance naming
// the commit + submodules populated) — the ONE predicate the immutable-ref fast path uses too —
// and no entry claims a resolution it did not take from upstream.

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

// Cache TTLs. ONE default — the eval-batch reality: a 16-lane, multi-phase check run re-resolves
// the same refs hundreds of times, so a shorter window re-probes mid-batch (spec CHANGELOG
// 0.2026247.2350: the former 5-minute window re-probed GitHub mid-batch — measured 152 concurrent
// `git ls-remote` -> throttling -> the deploy-add phase stalled at 493s+ with zero CPU/RAM
// pressure).
//
// It bounds ONLY the two questions whose answer is a NAME (a tag list; a default branch name) and,
// for latest_tags, only the IN-PROCESS half: the persisted latest_tags entries are the OFFLINE
// FALLBACK (served on fetch failure only), never TTL-fresh data.
//
// The question whose answer is CONTENT that can move upstream — which commit does this ref name
// NOW — is not bounded by a clock at all: see the struct's `downloads` field.
const (
	DefaultRefsCacheTTL = time.Hour
	LatestTagTTL        = DefaultRefsCacheTTL
	DefaultBranchTTL    = DefaultRefsCacheTTL
)

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
	// values winning). The former model — serving persisted entries as fresh for
	// the 1h TTL — was the stale-latest-tag bug: a fresh process minutes after a
	// tag push served the still-cached old tag.
	persistedTags   map[string]gitCacheEntry
	defaultBranches map[string]gitCacheEntry
	// downloads is the IN-PROCESS memo of resolved repo-export paths
	// (repoPath@version -> path): the batch-dedupe layer for the resolution question,
	// so one command (and every child of one wave) pays a given ref's re-resolution
	// once. It is deliberately NOT persisted.
	//
	// WHY NOT PERSISTED. A resolution's durable record is the export's OWN v2
	// provenance sidecar (RepoCacheProvenance), which names the commit the export was
	// cloned from — a resolved ref's CONTENT identity. Persisting a path beside a time
	// validity instead made the resolution a SECOND and weaker authority on the same
	// question: `os.Stat().IsDir()` answered "is this ref resolved?" ahead of
	// downloadRepoFrom's content check (`GitResolveRef` → `repoCacheFresh(path, commit)`
	// → re-clone on mismatch). A persisted entry could therefore serve an export that
	// no longer certified anything, with zero upstream contact, for a whole TTL —
	// measured live 2026-10-07: 0 `git ls-remote` calls and the deleted provenance
	// sidecar never restored (opencharly/charly#715, #530).
	//
	// The cost of dropping the durable half is one `git ls-remote` per (repo, mutable
	// ref) per PROCESS — paid only for MUTABLE refs, since an immutable tag or SHA
	// short-circuits network-free on IsRepoCached. The measured harm the former 1h
	// window bought (spec CHANGELOG 0.2026247.2350: 152 concurrent `git ls-remote`
	// mid-batch) is the WITHIN-batch re-probing this memo already removes; the
	// cross-process half is what a persisted entry could only ever deliver by not
	// asking upstream at all.
	downloads map[string]string
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
		downloads:       map[string]string{},
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
	return g.cacheFile, len(g.mergedLatestTags()) + len(g.defaultBranches)
}

// ClearCache drops every cached git answer (the in-memory entries and the persisted
// file), so the next resolutions are fresh — the `charly cache clear/refresh` surface.
func (g *GitClient) ClearCache() error {
	g.mu.Lock()
	g.latestTags = map[string]gitCacheEntry{}
	g.persistedTags = map[string]gitCacheEntry{}
	g.defaultBranches = map[string]gitCacheEntry{}
	g.downloads = map[string]string{}
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

	// Build the cache: git: {bypass, latest_tags, default_branches}. The former
	// resolved_refs/downloads keys are GONE: a resolved ref's durable record is the
	// export's own v2 provenance sidecar, not a path beside a timestamp (see the
	// `downloads` field's doc).
	gitVal := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "bypass"},
		{Kind: yaml.ScalarNode, Value: strconv.FormatBool(g.bypass)},
		{Kind: yaml.ScalarNode, Value: "latest_tags"},
		entryMapNode(g.mergedLatestTags()),
		{Kind: yaml.ScalarNode, Value: "default_branches"},
		entryMapNode(g.defaultBranches),
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
//
// DefaultRefsCacheTTL (the const block above documents the one-default reality), but a
// future per-question freshness divergence splits the constants without touching this function.
//
//nolint:unparam // ttl stays a parameter BY DESIGN: every TTL currently aliases
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
// Freshness contract (the offline-fallback cutover): the IN-PROCESS cache is the
// only fresh-data cache (batch dedupe within one process — the 152-concurrent-
// ls-remote throttling storm was a within-one-process fanout, solved by this
// layer). The PERSISTED latest_tags entries are an OFFLINE FALLBACK, never fresh
// data: a fresh process with a reachable network re-probes, so a tag pushed
// minutes ago is always seen. (The former model — serving persisted entries as
// fresh for the 1h TTL — was the stale-latest-tag bug: a fresh run resolved
// plugin refs to a tag list up to an hour old.) The persisted entry is served
// ONLY when the network fetch fails, with a loud stderr line — resilience, not
// freshness.
func (g *GitClient) LatestTag(repoURL string) (string, error) {
	g.mu.Lock()
	if !g.disabled {
		if v := cached(g.latestTags, repoURL, LatestTagTTL); v != "" {
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
		if v := cached(g.defaultBranches, repoURL, DefaultBranchTTL); v != "" {
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

// (ResolveRef was DELETED here: it produced no result any caller consumed — org-wide, no
// production call site existed, only the unrelated spec.WalkSeams.ResolveRef seam and
// net/url's ResolveReference — while carrying a second persisted, time-validated map over the
// same "which commit does this ref name" question. The live answer is GitResolveRef, which
// downloadRepoFrom already runs on every mutable-ref access; a persisted duplicate of it is
// precisely the content-blind authority this cutover removes (R3/R5).)

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
		have := (cached(g.latestTags, u, LatestTagTTL) != "" || g.persistedTags[u].Value != "") &&
			cached(g.defaultBranches, u, DefaultBranchTTL) != ""
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
// CACHED with a bounded freshness window (ResolveRefTTL) — the mutable-ref question ("has this
// branch moved upstream?") is inherently a NETWORK question, so it carries an explicit, declared
// bound rather than a content claim; everything else about the entry is content-addressed.
//
// A mutable ref (a branch or the default branch) can move, so the freshness contract requires
// re-resolving it: the miss path below IS `downloadRepoFrom`, which resolves the ref's CURRENT
// commit (`GitResolveRef`), refuses a stale export (`repoCacheFresh(cachePath, commit)`), re-clones
// it and re-stamps the provenance. Within one process the memo below means a command run twice in
// quick succession (e.g. the status fan-out resolving the envelope multiple times) pays that
// resolution once — that is the batch-dedupe the layer exists for, and it needs no time validity
// because a process is one moment in time. Across processes there is NO memo: a durable entry could
// only ever answer "has this ref moved upstream?" by not asking, which is the defect this cutover
// removes.
//
// The cached path is additionally validated against its CONTENT (IsCertifiedExport): an export that
// can no longer certify what it holds — a wiped, evicted, half-populated or provenance-less
// repo-cache dir — is never served (the content-validity principle — the same class the
// materialized-tree cache fixed with component drift detection, and the same predicate the
// immutable-ref fast path uses).
func (g *GitClient) Download(repoPath, version string, download func(repoPath, version string) (string, error)) (string, error) {
	key := repoPath + "@" + version
	g.mu.Lock()
	// The bypass is honored here as everywhere else: it is documented as "disables every cached
	// lookup" AND as the operator's on-demand truth for "a moved branch", and this is precisely the
	// moved-branch cache. It read the flag on no arm before this cutover.
	if !g.disabled {
		if v, ok := g.downloads[key]; ok && IsCertifiedExport(v) {
			g.mu.Unlock()
			return v, nil
		}
	}
	g.mu.Unlock()

	path, err := download(repoPath, version)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	g.downloads[key] = path
	g.mu.Unlock()
	return path, nil
}

// DownloadResult is the memo's read-only view for tests and diagnostics.
func (g *GitClient) DownloadResult(repoPath, version string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v, ok := g.downloads[repoPath+"@"+version]
	return v, ok
}
