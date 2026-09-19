package proc

// repo_override.go — RDD local-override plumbing (#55 W3 B2-full, promoted from
// charly/refs.go). RepoOverrideEnv / SelfSuperprojectOverridePair / MergeRepoOverrides were
// core-private, but the "stays core" framing was a domain claim, not a process-boundary one: both
// functions are pure `git`-shelling + string manipulation over spec.RootRepoIdentity, with zero
// registry coupling — freely callable from any process on the host. Promoted here (spec/proc
// already hosts RunCharlySubcommand, the sibling process-fabric primitive) so BOTH charly core
// (charly/plugin_loader.go's deployNodePluginContext, itself relocated from check_cmd.go at #55 W3
// B3) and a compiled-in plugin (candy/plugin-check's bed session, which computes its OWN
// repo-override before self-loading the project) share ONE implementation instead of the plugin
// needing a second copy.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencharly/spec/spec"
)

// RepoOverrideEnv configures RDD local-overrides: it points a remote `@github`
// repo ref at a LOCAL working tree (Go-`replace`-style), so an UNCOMMITTED
// candy / charly.yml change can be built and `charly check`'d by ANY
// consumer — across submodule boundaries — BEFORE it is committed and pushed.
// This is the supported "verify before you push to main" mechanism (no cache
// hacks, no producer-first tag churn).
//
// Value: a comma-separated list of `repoPath=localDir` pairs. repoPath matches
// the repo-root form every `@github` candy/namespace/image ref resolves through
// (`github.com/<org>/<repo>`); a bare `<org>/<repo>` is accepted too (auto
// `github.com/` prefix, same rule as `--repo`). Example:
//
//	CHARLY_REPO_OVERRIDE=opencharly/charly=/home/me/oc-charly \
//	    charly -C box/ubuntu box build ubuntu-coder
//
// The matched directory resolves verbatim (leading `~/` expanded); the ref's
// `:vTAG` is IGNORED — an override ALWAYS resolves to the dev's current tree.
const RepoOverrideEnv = "CHARLY_REPO_OVERRIDE"

// normalizeOverrideRepoPath canonicalizes the LHS of a CHARLY_REPO_OVERRIDE pair to
// the repo-root form spec.ParseRemoteRef yields, so `opencharly/charly` and
// `github.com/opencharly/charly` both match (same auto-prefix rule as
// spec.NormalizeRepoSpec).
func normalizeOverrideRepoPath(rp string) string {
	rp = strings.TrimSpace(strings.TrimSuffix(rp, "/"))
	if i := strings.Index(rp, "/"); i > 0 && !strings.Contains(rp[:i], ".") {
		return "github.com/" + rp
	}
	return rp
}

// RepoOverrideDir returns the configured local override directory for repoPath, or
// ("", false, nil) when none applies. envValue is the raw CHARLY_REPO_OVERRIDE value
// (a comma-separated list of `repoPath=localDir` pairs). A malformed entry, a
// missing/empty directory, or a non-directory target is a hard error — the override
// was set deliberately, so a typo must fail loud rather than silently fall through to
// a remote fetch.
//
// This is THE single implementation of the CHARLY_REPO_OVERRIDE parse: the
// comma-separated repoPath=localDir split, the repo-path normalization (a bare
// owner/repo LHS auto-prefixes github.com, the same rule as spec.NormalizeRepoSpec),
// the `~/` home expansion, and the exists-and-is-a-directory check.
//
// It lives HERE, in spec/proc, next to RepoOverrideEnv, so BOTH the loader-side
// orchestration (sdk/loaderkit.EnsureRepoDownloaded) AND the fetch LEAF
// (spec/refs.DownloadRepo, used directly by `charly marketplace generate`,
// `charly docs generate`, and pluginsgen) share ONE parse. Before this move the parse
// lived only in loaderkit, so every direct DownloadRepo caller silently bypassed the
// override — an R4 hole (the mechanism exists to verify uncommitted work before
// pushing; a leaf that ignores it defeats it). A consumer MUST call this rather than
// re-parse the env value (R3 — one canonical implementation per behavior).
func RepoOverrideDir(repoPath, envValue string) (string, bool, error) {
	envValue = strings.TrimSpace(envValue)
	if envValue == "" {
		return "", false, nil
	}
	for pair := range strings.SplitSeq(envValue, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.LastIndex(pair, "=")
		if eq < 0 {
			return "", false, fmt.Errorf("CHARLY_REPO_OVERRIDE: malformed entry %q (want repoPath=localDir)", pair)
		}
		if normalizeOverrideRepoPath(pair[:eq]) != repoPath {
			continue
		}
		dir := strings.TrimSpace(pair[eq+1:])
		if dir == "" {
			return "", false, fmt.Errorf("CHARLY_REPO_OVERRIDE: empty directory for repo %q", repoPath)
		}
		if strings.HasPrefix(dir, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, dir[2:])
			}
		}
		info, err := os.Stat(dir)
		if err != nil {
			return "", false, fmt.Errorf("CHARLY_REPO_OVERRIDE: override dir for %q not accessible: %w", repoPath, err)
		}
		if !info.IsDir() {
			return "", false, fmt.Errorf("CHARLY_REPO_OVERRIDE: override for %q is not a directory: %s", repoPath, dir)
		}
		return dir, true, nil
	}
	return "", false, nil
}

// SelfSuperprojectOverridePair returns a CHARLY_REPO_OVERRIDE pair
// (`<repo-identity>=<superproject-dir>`) that points a bed project's OWN
// superproject `@github` refs at the local working tree, or "" when projectDir
// is not a git submodule of a charly superproject. A check bed (a `disposable: true` deploy) living in
// a `box/<distro>` submodule references its parent repo's shared candies via
// `@github.com/<org>/<parent>/candy/<name>:<tag>`; without this override the bed
// would build the PINNED REMOTE candy and so test STALE code — the candy-ref
// analogue of why the bed runner builds the toolchain with `--dev-local-pkg`. The
// override IGNORES the ref's `:vTAG`, so the bed always tests the dev's current
// tree. Returns "" when projectDir is its own root (its candies already resolve
// from the local tree) or when git / the superproject identity is unavailable.
func SelfSuperprojectOverridePair(projectDir string) string {
	out, err := exec.Command("git", "-C", projectDir, "rev-parse", "--show-superproject-working-tree").Output()
	if err != nil {
		return ""
	}
	superDir := strings.TrimSpace(string(out))
	if superDir == "" {
		return "" // not a submodule — its candies already resolve from the local tree
	}
	identity := spec.RootRepoIdentity(superDir)
	if identity == "" {
		return ""
	}
	return identity + "=" + superDir
}

// MergeRepoOverrides combines an existing CHARLY_REPO_OVERRIDE value with an
// auto-added pair. The existing (operator-set) entries are placed FIRST so an
// explicit operator override for a repo WINS over the auto pair — repoOverrideDir
// returns the FIRST matching entry. Either argument may be empty.
func MergeRepoOverrides(existing, add string) string {
	existing = strings.TrimSpace(existing)
	add = strings.TrimSpace(add)
	switch {
	case existing == "":
		return add
	case add == "":
		return existing
	default:
		return existing + "," + add
	}
}
