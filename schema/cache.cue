// cache.cue — the `cache:` kind: LOCAL cache status persisted in the per-host
// charly.yml (~/.config/charly/charly.yml).
//
// The per-host config is the SINGLE home for local system state — deployments
// (`deploy:`), install records (`ledger:`), local system info (`system:`), and
// cache status (`cache:`) — so the git metadata cache the loader resolves
// @github refs with lives HERE, not in ad-hoc JSON files under
// ~/.cache/charly/repos (git-cache.json / latest-tags.json, both deleted by the
// GitClient cutover). One file, one schema, one validation path.

// #CacheConfig is the top-level `cache:` block. `git` holds the git metadata
// cache; future caches (image layers, check runs) add their own sub-key.
#CacheConfig: {
	git?: #GitCache @go(Git)
}

// #GitCache is the git metadata cache: latest tags and default branches. Each entry
// records the value and when it was resolved (RFC3339), so the TTL policy can decide
// freshness.
//
// `resolved_refs` and `downloads` were DELETED with the resolved-ref identity
// cutover: both persisted an answer to "which commit does this ref name NOW" beside a
// timestamp, so a MUTABLE pin that had moved upstream was served stale for the whole
// window without ever asking (opencharly/charly#715, #530). A resolved ref's durable
// record is its export's own v2 provenance sidecar (`refs.RepoCacheProvenance`), which
// names the commit it was cloned from — its CONTENT identity — and `GitClient.Download`
// re-takes the upstream answer once per process. Do not reintroduce a persisted
// resolution key here: a cache whose validity is a clock, not a content digest, is the
// defect that cutover removed.
#GitCache: {
	bypass?:          bool                   @go(Bypass)
	latest_tags?:      {[string]: #GitCacheEntry} @go(LatestTags)
	default_branches?: {[string]: #GitCacheEntry} @go(DefaultBranches)
}

// #GitCacheEntry is one cached git answer: the value plus the resolution time.
#GitCacheEntry: {
	value!:    string
	resolved!: string
}
