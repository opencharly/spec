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
//
// This is the WHOLE-PROJECT entry (fetchProject): the export carries every
// submodule the repo declares. A files-only export is a DIFFERENT entry — see
// repoFilesCachePath — because the two exports hold different content, and a
// cache key has to name what it holds.
func RepoCachePath(repoPath, version string) (string, error) {
	cacheDir, err := RepoCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, repoPath+"@"+version), nil
}

// filesCacheDir is the SUBDIRECTORY holding the files-only exports, one level below
// RepoCacheDir. It is what makes a files-only entry a DIFFERENT cache key from the
// whole-project one, so neither fetch can ever be served the other's content and
// neither has to invalidate the other.
//
// A mode DIRECTORY, not a filename suffix — deliberately, and for a measured
// reason: the entry's BASENAME stays `<repoPath>@<version>`, which is the shape
// consumers read the source tag out of. plugin-marketplace's resolve.go ranks two
// sources publishing one identity by the CalVer at the END of Entity.SourceRoot
// (`@v<YYYY>.<DDD>.<HHMM>$`); a `.files` suffix on the basename made every fetched
// source unparseable, and an unparseable rank silently hands the identity to
// whichever twin still parsed. The dot prefix keeps the directory from ever
// colliding with a host directory name (`github.com`, `gitlab.com`, …).
const filesCacheDir = ".files"

// repoFilesCachePath returns the cache path for a repo version's FILES-ONLY
// export (fetchFiles).
// e.g. ~/.cache/charly/repos/.files/github.com/org/repo@v1.0.0
func repoFilesCachePath(repoPath, version string) (string, error) {
	dir, err := RepoCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filesCacheDir, repoPath+"@"+version), nil
}

// repoCachePathFor returns the cache entry a fetchMode publishes to: the
// whole-project path or the files-only one.
func repoCachePathFor(repoPath, version string, mode fetchMode) (string, error) {
	if mode == fetchFiles {
		return repoFilesCachePath(repoPath, version)
	}
	return RepoCachePath(repoPath, version)
}

// IsRepoCached reports whether a repo version is already in the cache AS A
// USABLE EXPORT — the directory exists, carries V2 provenance, AND every
// submodule it declares has content.
//
// It answers the WHOLE-PROJECT question only (fetchProject): the entry it judges
// is RepoCachePath's, the one a `--repo` drive reads. A files-only export is a
// different entry with a different, smaller contract (repoFilesCachePath), and
// its freshness is decided by its own fetch (repoCacheFreshMode) — never by this
// predicate, whose submodule requirement such an export could not satisfy by
// design.
//
// The completeness half is load-bearing for IMMUTABLE refs. EnsureRepoDownloaded
// short-circuits on `cached && !IsMutableRef(version)`, returning RepoCachePath
// directly and never entering DownloadRepo — so the repoCacheFreshMode completeness
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
	_, err = os.Stat(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, ok := ReadRepoCacheProvenance(cachePath); !ok {
		return false, nil
	}
	return submodulesPopulated(cachePath), nil
}
