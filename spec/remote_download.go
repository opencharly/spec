package spec

// RemoteDownload represents a unique (repo, version) pair to download in the remote-layer resolver's
// candy-scan fix-point, plus the bare refs to import from it. A pure data descriptor over strings,
// relocated to the dedicated spec module (#55 2b Class A) so charly's remote-resolver files (refs.go)
// reach it without importing loaderkit; loaderkit aliases it for the scan mechanism that produces it.
type RemoteDownload struct {
	RepoPath string
	Version  string
	Refs     []string // bare refs to import (e.g. "github.com/org/repo/candy/name")
	// RefReferrers maps each bare ref in Refs to the SCOPE labels that named it
	// ("box=<name>", "layer=<name>", "kind:local=<tpl>", "deploy=add_candy"). The ref
	// collector sees the reachability context; the post-fetch arbiter
	// (loaderkit.PickCandyVersion) does not, so the context travels WITH the fetch descriptor
	// and is attached to each spec.CandyCandidate it produces. It is what lets the arbiter
	// distinguish a genuine SAME-scope conflict (≥2 referrers sharing a scope) from
	// independent boxes that legitimately pin different versions (no notice).
	//
	// Optional: a producer that has no referrer context (a synthetic/test download) leaves it
	// nil and its candidates are treated as scope-less, exactly as the pre-scoping behaviour.
	RefReferrers map[string][]string
}
