package refs

import (
	"fmt"
	"os"
	"path/filepath"
)

// cache.go — the remote-repo cache LOCATION helpers. Pure path computation over
// $CHARLY_REPO_CACHE / ~/.cache/charly/repos, shared by core (reconcile / the collection walk)
// and the refs fetch backend (candy/plugin-refs) that clones into these paths. RELOCATED from
// sdk/kit alongside the git primitives (#55 fabric-primitive extraction).

// RepoCacheDir returns the cache directory for remote repos.
// Uses $CHARLY_REPO_CACHE env var if set, otherwise ~/.cache/charly/repos/.
func RepoCacheDir() (string, error) {
	if envDir := os.Getenv("CHARLY_REPO_CACHE"); envDir != "" {
		return envDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "charly", "repos"), nil
}

// RepoCachePath returns the cache path for a specific repo version.
// e.g. ~/.cache/charly/repos/github.com/org/repo@v1.0.0/
func RepoCachePath(repoPath, version string) (string, error) {
	cacheDir, err := RepoCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, repoPath+"@"+version), nil
}

// IsRepoCached reports whether a repo version is already in the cache AS A
// USABLE EXPORT — the directory exists, carries V2 provenance, AND every
// submodule it declares has content.
//
// The completeness half is load-bearing for IMMUTABLE refs. EnsureRepoDownloaded
// short-circuits on `cached && !IsMutableRef(version)`, returning RepoCachePath
// directly and never entering DownloadRepo — so the repoCacheFresh completeness
// check cannot see a tag's cache at all. Directory-exists alone therefore pinned
// every incomplete TAG export permanently: a tag never moves, so nothing would
// ever re-fetch it. On the machine this was found, `charly@v2026.183.1359` held
// 0 of its 9 declared submodules and `charly@v2026.201.0706` held 1 of 10, both
// unrepairable for the life of the cache. Checking content here routes an
// incomplete export down the download branch instead, so tag and branch caches
// self-heal by the SAME predicate rather than one growing its own copy.
//
// The provenance half is what heals a POLLUTED cache. A v1 (legacy bare-commit)
// sidecar cannot certify its tree — the pre-cutover loader could rewrite the
// export in place (auto-migrating charly.yml to the consumer's schema CalVer) —
// so a legacy export is treated as not-cached and re-fetched once, restoring
// pristine content. This is network-free (a sidecar read), so the immutable-ref
// short-circuit still avoids a fetch on every healthy tag access.
func IsRepoCached(repoPath, version string) (bool, error) {
	cachePath, err := RepoCachePath(repoPath, version)
	if err != nil {
		return false, err
	}
	if _, statErr := os.Stat(cachePath); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, statErr
	}
	return IsCertifiedExport(cachePath), nil
}

// IsCertifiedExport reports whether cachePath holds a repo export that can CERTIFY its own
// content — the ONE content-validity predicate for a resolved ref's materialization, shared by
// EVERY path that decides whether a cached export may be served (R3).
//
// An export is certified iff it is a directory AND carries v2 provenance
// (RepoCacheProvenance — the commit it was cloned from: its CONTENT identity, not its path) AND
// every submodule it declares has content on disk. Path existence alone is NOT validity: a
// wiped, half-populated or provenance-less export has a directory too, and serving it silently
// breaks the "content validity, never marker presence" invariant the resolved-ref identity
// rests on.
//
// Two paths previously answered this question with two DIFFERENT predicates on the SAME object:
// the immutable-ref fast path (IsRepoCached) asked for content, while the ref-resolution cache
// (GitClient.Download) asked only `os.Stat().IsDir()` — strictly weaker. A persisted resolution
// whose export had lost its provenance sidecar was therefore served for the whole TTL with ZERO
// upstream contact (measured live 2026-10-07: 0 `git ls-remote` calls, the `.ref` sidecar never
// restored, the content-invalid export served). One object, one predicate.
func IsCertifiedExport(cachePath string) bool {
	st, err := os.Stat(cachePath)
	if err != nil || !st.IsDir() {
		return false
	}
	if _, ok := ReadRepoCacheProvenance(cachePath); !ok {
		return false
	}
	return submodulesPopulated(cachePath)
}
