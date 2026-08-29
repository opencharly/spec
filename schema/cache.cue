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

// #GitCache is the git metadata cache: latest tags, default branches, resolved
// refs, and repo downloads. Each entry records the value and when it was
// resolved (RFC3339), so the TTL policy can decide freshness.
#GitCache: {
	latest_tags?:      {[string]: #GitCacheEntry} @go(LatestTags)
	default_branches?: {[string]: #GitCacheEntry} @go(DefaultBranches)
	resolved_refs?:    {[string]: #GitCacheEntry} @go(ResolvedRefs)
	downloads?:        {[string]: #GitCacheEntry} @go(Downloads)
}

// #GitCacheEntry is one cached git answer: the value plus the resolution time.
#GitCacheEntry: {
	value!:    string
	resolved!: string
}
