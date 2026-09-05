// Package refs is the spec fabric slice for the remote-repo GIT primitives — pure git-exec +
// string parsing + cache-path computation, host primitives a plugin composes but the kernel
// itself needs (reconcile's --remote tag query, the collection walk). RELOCATED from sdk/kit
// (#55 fabric-primitive extraction), so charly core reaches them via spec/refs directly and the
// remaining sdk/candy callers keep a thin kit re-export. The FETCH ORCHESTRATION (override →
// cache → clone → migrate) still lives in the refs plugin (candy/plugin-refs); these are the
// reusable mechanisms it composes. The only non-stdlib dependency is the advisory file lock
// (spec/lock) DownloadRepo serializes concurrent cache refresh with.
package refs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opencharly/spec/cache"
	"github.com/opencharly/spec/calver"
	"github.com/opencharly/spec/lock"
	"github.com/opencharly/spec/spec"
)

// ---------------------------------------------------------------------------
// Bounded git-op runner: every NETWORK git subprocess in this package executes
// through runGitOp — a context deadline plus a small bounded retry.
//
// Before this, a `git ls-remote`/`git fetch` whose connection died in a GitHub
// HTTP/2 reset window (curl error 92 CANCEL / "RPC failed; HTTP/2 stream 5
// reset") blocked its caller FOREVER: exec.Command carries no deadline and git's
// libcurl starts with no timeout, so the pre-deploy @github ref-resolution hung
// indefinitely on a dead socket (r9 wave forensics: three 16-lane eval check
// runs froze before deploy-add — 21 threads, 20x futex_wait + 1x do_epoll_wait
// on the killed connection). The deadline turns the hang into an error; the
// bounded retry re-issues the op so a transiently-killed connection re-resolves
// instead of hanging a lane forever.
//
// Deadline split: ref METADATA (ls-remote) is a tiny request-response — 30s is
// a generous bound. TRANSFER (clone/fetch/submodule init) moves real bytes — a
// 60s bound still fires on a stalled connection while leaving a legitimately
// slow-but-moving transfer room. Both are variables (not constants) only so the
// tests can shrink them; production code never mutates them.
// ---------------------------------------------------------------------------

var (
	gitOpMetadataTimeout = 30 * time.Second // ref metadata: ls-remote (resolve/latest-tag/default-branch)
	gitOpTransferTimeout = 60 * time.Second // transfer: clone / fetch / submodule update
	gitOpRetries         = 2                // bounded retries AFTER the first attempt
	gitOpBackoff         = 500 * time.Millisecond
)

// gitOpTransientRe matches git's stderr signatures for a connection killed in a
// reset window or otherwise-flaky network — the exact curl-error-92 /
// "RPC failed; HTTP/2 stream 5 reset" class that introduced this runner. A
// permanently-missing repo ("Repository not found", URL error 404/403) does NOT
// match and is NOT retried: retrying a permanent failure would mask the real
// error and triple its latency.
var gitOpTransientRe = regexp.MustCompile(
	`(?i)(rpc failed|http/2|connection (reset|refused|closed|aborted)|could not resolve host|failed to connect to|timed out|early eof|network is unreachable|transfer closed with outstanding|hung up unexpectedly|unexpected disconnect|empty reply from server|returned error: 5\d\d)`,
)

// runGitOp executes one `git` subprocess under a context deadline with a bounded
// retry. transfer selects the transfer (60s) vs metadata (30s) timeout; dir is
// the working directory ("" = process cwd); tee (may be nil) receives git's
// stderr live (clone progress) IN ADDITION to the capture that is folded into
// the returned error on failure. Returns the stdout bytes of the first attempt
// that completes within the deadline. A permanent failure (non-transient error)
// fails fast without retrying.
func runGitOp(transfer bool, dir string, tee io.Writer, args ...string) ([]byte, error) {
	timeout := gitOpMetadataTimeout
	if transfer {
		timeout = gitOpTransferTimeout
	}
	var (
		out        []byte
		lastErr    error
		lastDetail string
		timedOut   bool
	)
	for attempt := 0; attempt <= gitOpRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(gitOpBackoff * time.Duration(attempt)) // short backoff: 1x, 2x
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		if tee != nil {
			cmd.Stderr = io.MultiWriter(&stderr, tee)
		} else {
			cmd.Stderr = &stderr
		}
		var err error
		out, err = cmd.Output()
		// Capture the deadline verdict BEFORE cancel(): after cancel, `ctx.Err()` is
		// always Canceled even when the deadline fired, which would otherwise hide
		// the hang case and, worse, skip the stderr-signature check for fast
		// failures (a killed connection reports "Empty reply from server" etc.
		// before any deadline).
		timedOut = ctx.Err() == context.DeadlineExceeded
		cancel()
		if err == nil {
			return out, nil
		}
		lastErr, lastDetail = err, strings.TrimSpace(stderr.String())
		if !timedOut && !gitOpTransientRe.MatchString(lastDetail) {
			break // permanent failure — retrying cannot help
		}
	}
	wrapped := lastErr
	if timedOut {
		wrapped = fmt.Errorf("timed out after %s", timeout)
	}
	// Bare error: every caller wraps with its own "git <op> <url>" context
	// (e.g. GitLatestTag: "git ls-remote --tags %s"), so a prefix here would
	// duplicate (the pre-runGitOp callers already name the op).
	if lastDetail != "" {
		return nil, fmt.Errorf("%w\n%s", wrapped, lastDetail)
	}
	return nil, wrapped
}

// GitResolveRef resolves a git reference (tag, branch, or commit) to a full commit hash.
// Uses git ls-remote for tags/branches; for commit hashes, validates length and returns as-is.
//
// NOTE: deliberately NOT cached. DownloadRepo's freshness contract resolves the ref to its
// CURRENT commit on every call, so a mutable branch (main) that moved upstream is re-downloaded
// instead of serving stale content (refs/git_test.go TestDownloadRepoFrom_RefreshesMovedBranch).
// A TTL cache here would break that contract — the freshness check must see the live commit.
func GitResolveRef(repoURL string, ref string) (string, error) {
	// If ref looks like a full commit hash (40 hex chars), return as-is
	if len(ref) == 40 && isHex(ref) {
		return ref, nil
	}

	// Query the ref AND its peeled ^{} form so an ANNOTATED tag resolves to the
	// underlying COMMIT (refs/tags/X^{}), not the tag object (refs/tags/X).
	out, err := runGitOp(false, "", nil, "ls-remote", repoURL, ref, "refs/tags/"+ref, "refs/tags/"+ref+"^{}", "refs/heads/"+ref)
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s %s: %w", repoURL, ref, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if commit := pickResolvedCommit(lines, ref); commit != "" {
		return commit, nil
	}

	// If nothing matched but ref is a short hex, it might be a short commit
	if len(ref) >= 7 && isHex(ref) {
		return ref, nil
	}

	return "", fmt.Errorf("could not resolve ref %q in %s", ref, repoURL)
}

// pickResolvedCommit selects the commit hash for ref from `git ls-remote`
// output lines. An ANNOTATED tag exposes two refs — `refs/tags/<ref>` (the tag
// OBJECT) and `refs/tags/<ref>^{}` (the COMMIT it points at) — so the peeled
// form is preferred; a lightweight tag or branch has only its direct ref.
// Returns "" when ref isn't present. Keeping this pure makes the peel-preference
// unit-testable without a network round-trip.
func pickResolvedCommit(lines []string, ref string) string {
	peeled := "refs/tags/" + ref + "^{}"
	tagRef := "refs/tags/" + ref
	headRef := "refs/heads/" + ref
	// 1. Peeled annotated-tag commit wins (never the tag object).
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == peeled {
			return parts[0]
		}
	}
	// 2. Exact lightweight-tag / branch / ref match.
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && (parts[1] == tagRef || parts[1] == headRef || parts[1] == ref) {
			return parts[0]
		}
	}
	// 3. Defensive: any other peeled ref present.
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.HasSuffix(parts[1], "^{}") {
			return parts[0]
		}
	}
	return ""
}

// GitClone clones a git repository at a specific ref into the target directory.
// Uses shallow clone for efficiency.
func GitClone(repoURL string, ref string, commit string, targetDir string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Primary: shallow-clone the resolved COMMIT directly. `commit` is already
	// peeled to a real commit by GitResolveRef, and GitHub/GitLab allow fetching
	// a reachable commit by sha. This avoids git's
	// "refs/tags/<tag> <sha> is not a commit!" warning that
	// `git clone --depth 1 --branch <annotated-tag>` emits (the annotated tag
	// ref is a tag object, not a commit).
	if len(commit) >= 7 && isHex(commit) {
		if err := gitCloneByCommit(repoURL, commit, targetDir); err == nil {
			return populateSubmodules(targetDir)
		}
		_ = os.RemoveAll(targetDir) // clean up partial clone before falling back
	}

	// Fallback: clone by ref name (servers that don't allow fetch-by-sha).
	if _, err := runGitOp(true, "", os.Stderr, "clone", "--depth", "1", "--branch", ref, repoURL, targetDir); err != nil {
		_ = os.RemoveAll(targetDir) // clean up partial clone
		return fmt.Errorf("git clone --branch %s %s: %w", ref, repoURL, err)
	}

	return populateSubmodules(targetDir)
}

// populateSubmodules initializes EVERY submodule a freshly-fetched repo declares.
// The raw clone/fetch above populates none of them, and a `--repo` cache is a
// WHOLE PROJECT a user drives with nothing but the charly binary — so any
// submodule may be load-bearing for a documented command:
//
//   - sdk + spec — every out-of-process plugin candy builds STANDALONE in its own
//     module (GOWORK=off) against `replace … => ../../sdk` and `=> ../../spec`.
//     ALL 86 candy go.mod files carry BOTH replaces, so populating only `sdk`
//     left every such build failing on `../../spec/go.mod: no such file`, and no
//     out-of-process verb could be reached through --repo at all.
//   - box/<distro> — the box definitions themselves; main owns none.
//   - plugins, docs, pkg/* — skills, the docs site, and the packaging sources.
//
// This deliberately replaces an `sdk`-only special case whose stated rationale
// ("the ONLY submodule a plugin build depends on … never the heavy box/* ones")
// was wrong on both counts: spec is equally required, and a shallow init of all
// twelve costs ~8s and adds 24MB to a 36MB clone (60MB total) — the cost that
// "heavy" was guarding against does not exist at --depth 1. A repo declaring no
// submodules is a clean no-op.
//
// The insteadOf rewrite forces the .gitmodules SSH URL (git@github.com:) to
// HTTPS — matching how the parent repo is cloned — so no SSH key is needed in a
// headless/CI run. Non-recursive, matching the configuration proven to build.
//
// A SUBMODULE THAT CANNOT BE FETCHED FAILS THE WHOLE CLONE, deliberately. This
// is a real widening: the old code was a silent no-op for any repo not
// declaring `sdk`, so a third-party project with a private or dead submodule
// used to fetch "fine" and now errors here. That is the correct trade, because a
// SILENTLY PARTIAL cache is the exact defect this function exists to fix — it
// cost a full debugging session, surfacing three layers down as
// `../../spec/go.mod: no such file` with nothing pointing back at the fetch. A
// caller that cannot reach a declared submodule has an incomplete project and is
// told so HERE, naming the submodule, rather than at some later build whose
// error does not mention fetching at all. git's own stderr is wrapped in, so the
// specific submodule and cause survive into the message.
//
// The blast radius is narrower than .gitmodules suggests: `submodule update
// --init` walks the INDEX, so an entry declared in .gitmodules with no gitlink
// (mode 160000) recorded is ignored and cannot fail a fetch. Only submodules
// genuinely committed into the tree are attempted.
func populateSubmodules(targetDir string) error {
	gm := filepath.Join(targetDir, ".gitmodules")
	if _, err := os.Stat(gm); err != nil {
		return nil // no submodules declared — nothing to populate
	}
	if _, err := runGitOp(true, targetDir, nil,
		"-c", "url.https://github.com/.insteadOf=git@github.com:",
		"-c", "advice.detachedHead=false",
		"submodule", "update", "--init", "--depth", "1", "-q"); err != nil {
		// runGitOp captures git stderr into the error: git names the offending
		// submodule and the reason (auth, missing ref, unreachable host) — the only
		// thing that makes this failure actionable (the same reason the old code
		// captured rather than passed stderr through).
		return fmt.Errorf("populating submodules in %s: %w", targetDir, err)
	}
	return nil
}

// gitCloneByCommit clones a repo and checks out a specific commit
func gitCloneByCommit(repoURL string, commit string, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	cmds := [][]string{
		// Keep the throwaway clone SILENT on success (zero-warnings gate):
		// -c init.defaultBranch suppresses git's "using 'master' ... suppress
		// this warning" hint; -q + advice.detachedHead=false silence the
		// remaining init / fetch / detached-checkout chatter.
		{"-c", "init.defaultBranch=main", "init", "-q"},
		{"remote", "add", "origin", repoURL},
		{"fetch", "--depth", "1", "-q", "origin", commit}, // the ONE network step
		{"-c", "advice.detachedHead=false", "checkout", "-q", "FETCH_HEAD"},
	}

	for _, args := range cmds {
		// Only the fetch moves bytes across the network: it gets the transfer
		// (60s) deadline + retry; the local steps are routed through the same
		// bounded runner purely for uniformity — they cannot hang, and their
		// failures carry no transient stderr signature, so they never retry.
		transfer := args[0] == "fetch"
		if _, err := runGitOp(transfer, targetDir, os.Stderr, args...); err != nil {
			_ = os.RemoveAll(targetDir) // clean up on failure
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
	}

	return nil
}

// RepoGitURL converts a repo path to a git clone URL.
// e.g. "github.com/opencharly/ml-layers" -> "https://github.com/opencharly/ml-layers.git"
func RepoGitURL(repoPath string) string {
	return "https://" + repoPath + ".git"
}

// IsMutableRef reports whether a repo ref version can ADVANCE upstream (a
// branch name such as main, or an empty version that resolves to the default
// branch). Immutable coordinates — CalVer/semver tags (v…) and full commit
// SHAs — never change what they point at; the project's tags are add-only.
// Mutable refs must re-resolve on every access: a cache hit on a branch
// otherwise freezes it at first-download content forever (the pre-#146 @main
// protocol skew — a stale main export served protocol-v1 plugin sources
// indefinitely).
func IsMutableRef(version string) bool {
	if version == "" {
		return true
	}
	if strings.HasPrefix(version, "v") {
		return false
	}
	if len(version) == 40 && isHex(version) {
		return false
	}
	return true
}

// refProvenancePath is the sidecar recording the commit a cache export was
// cloned from.
func refProvenancePath(cachePath string) string {
	return cachePath + ".ref"
}

// writeRefProvenance records the clone's resolved commit next to the export.
func writeRefProvenance(cachePath, commit string) error {
	return os.WriteFile(refProvenancePath(cachePath), []byte(commit+"\n"), 0o644)
}

// repoCacheFresh reports whether the cache at cachePath is a complete export
// cloned from exactly commit. A missing export, a missing provenance sidecar
// (a cache written before this contract), a sidecar naming a different commit
// (the ref moved upstream), or an INCOMPLETE export all count as stale.
//
// Completeness is checked, not assumed. This function has always claimed to
// verify "a complete export" and never did — it compared only the commit — so a
// cache whose CONTENT was wrong stayed a permanent hit as long as the ref did
// not move. That is not hypothetical: every cache written before
// populateSubmodules holds one of twelve submodules, records the correct commit,
// and is therefore served forever. Without this check the submodule fix would
// only ever help caches created AFTER it, and every existing user would stay
// broken until main happened to advance — with no CLI verb to invalidate a repo
// cache entry (`charly clean --invalidate` targets image tags), leaving hand-
// deleting a cache directory as the only remedy. Verifying content instead makes
// every such cache self-heal on next access.
func repoCacheFresh(cachePath, commit string) bool {
	if commit == "" {
		return false
	}
	if st, err := os.Stat(cachePath); err != nil || !st.IsDir() {
		return false
	}
	recorded, err := os.ReadFile(refProvenancePath(cachePath))
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(recorded)) != commit {
		return false
	}
	return submodulesPopulated(cachePath)
}

// submodulesPopulatedCache caches the submodule-populated verdict per cache path.
// The verdict is stable for the process lifetime (the cache dirs do not change
// during a run), so a process-wide cache eliminates the repeated
// `git config -f .gitmodules` subprocess spawns that dominated `charly status`
// (one spawn per cached repo, measured). A PERSISTENT cache file extends this
// across runs — the repos are fetched once and cached, so the verdict does not
// change between runs either.
var submodulesPopulatedCache sync.Map // cachePath -> bool

// submodulesPopulated reports whether every submodule the export declares has
// content on disk. An export declaring none is trivially complete. Cached
// process-wide per cache path, with a persistent cache file across runs.
//
// The export has had its .git removed (see downloadRepoFrom), so this reads
// .gitmodules directly — via `git config -f`, the same parser git itself uses,
// rather than a second hand-rolled INI reader that could disagree with it. An
// unreadable or absent .gitmodules means nothing to verify, which keeps a
// non-submodule repo on the pure cache-hit path.
func submodulesPopulated(cachePath string) bool {
	if v, ok := submodulesPopulatedCache.Load(cachePath); ok {
		return v.(bool)
	}
	if v, ok := readSubmoduleCache(cachePath); ok {
		submodulesPopulatedCache.Store(cachePath, v)
		return v
	}
	result := submodulesPopulatedUncached(cachePath)
	submodulesPopulatedCache.Store(cachePath, result)
	writeSubmoduleCache(cachePath, result)
	return result
}

// submoduleCacheTTL is how long a cached submodule-populated verdict is trusted.
// The repo cache dirs change only on a re-fetch, so a 1-hour TTL makes
// consecutive status runs fast while still seeing a re-fetched repo within an
// hour.
const submoduleCacheTTL = time.Hour

// submoduleCacheValue is the cached verdict for one cache path.
type submoduleCacheValue struct {
	Populated bool `json:"populated"`
}

// submoduleCachePath returns the persistent submodule-verdict cache file under
// the charly dir (~/.config/charly/cache/submodules.json).
func submoduleCachePath() (string, error) {
	cfg, err := spec.DefaultDeployConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfg), "cache", "submodules.json"), nil
}

// readSubmoduleCache returns the cached verdict for cachePath if fresh, else
// (false, false). A corrupt/absent file is a cache miss.
func readSubmoduleCache(cachePath string) (bool, bool) {
	path, err := submoduleCachePath()
	if err != nil {
		return false, false
	}
	var v submoduleCacheValue
	if !cache.Read(path, cachePath, submoduleCacheTTL, &v) {
		return false, false
	}
	return v.Populated, true
}

// writeSubmoduleCache persists the verdict (best-effort).
func writeSubmoduleCache(cachePath string, populated bool) {
	path, err := submoduleCachePath()
	if err != nil {
		return
	}
	cache.Write(path, cachePath, submoduleCacheValue{Populated: populated})
}

func submodulesPopulatedUncached(cachePath string) bool {
	gm := filepath.Join(cachePath, ".gitmodules")
	if _, err := os.Stat(gm); err != nil {
		return true
	}
	out, err := exec.Command("git", "config", "-f", gm, "--get-regexp", `submodule\..*\.path`).Output()
	if err != nil {
		return true // no submodule.*.path entries — nothing to verify
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(cachePath, fields[1]))
		if err != nil {
			// ABSENT is not incomplete. git materializes an empty placeholder
			// directory for every GITLINK it clones, so a path missing entirely
			// was never gitlinked — a .gitmodules entry with no index entry, which
			// populateSubmodules cannot fetch either (it walks the index). Treating
			// that as incomplete would make the export permanently unfresh and
			// re-clone the repo on EVERY command, forever. Only an existing-but-
			// empty directory is the real unpopulated-gitlink case.
			continue
		}
		if len(entries) == 0 {
			return false
		}
	}
	return true
}

// DownloadRepo downloads a remote repo to the cache.
// Returns the cache path where the repo was stored.
//
// Freshness contract: the cache is reused ONLY when its recorded provenance
// (the commit the export was cloned from) equals the ref's CURRENTLY resolved
// commit — so a branch that advanced upstream (main) is re-downloaded instead
// of silently serving stale content. The check costs the one ls-remote
// GitResolveRef already performs; for immutable tags the provenance always
// matches, so their cache-hit behavior is unchanged. A cache written before
// this contract has no provenance and is re-downloaded once, then self-heals.
func DownloadRepo(repoPath string, version string) (string, error) {
	return downloadRepoFrom(RepoGitURL(repoPath), repoPath, version)
}

// downloadRepoFrom is DownloadRepo with the clone URL explicit, so the
// freshness contract is testable against a local file:// remote.
func downloadRepoFrom(repoURL, repoPath, version string) (string, error) {
	// Resolve the ref to a commit hash
	commit, err := GitResolveRef(repoURL, version)
	if err != nil {
		return "", fmt.Errorf("resolving %s:%s: %w", repoPath, version, err)
	}

	cachePath, err := RepoCachePath(repoPath, version)
	if err != nil {
		return "", err
	}

	// Serialize concurrent download/refresh of the SAME cache path (a blocking
	// per-path file lock, the plugin build-cache pattern): the loser re-checks
	// provenance under the lock and reuses the winner's fresh export instead of
	// cloning over it.
	release, err := lock.AcquireFileLock(cachePath+".lock", true)
	if err != nil {
		return "", fmt.Errorf("acquiring repo cache lock %s: %w", cachePath, err)
	}
	defer func() { _ = release() }()

	if repoCacheFresh(cachePath, commit) {
		return cachePath, nil
	}

	fmt.Fprintf(os.Stderr, "Downloading %s:%s...\n", repoPath, version)

	// Clone into a sibling temp dir first, then swap: readers see either the
	// complete old export or the complete new one, never a half-cloned tree.
	tmpPath := cachePath + ".download"
	_ = os.RemoveAll(tmpPath) // leftover from an interrupted earlier download
	if err := GitClone(repoURL, version, commit, tmpPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		return "", fmt.Errorf("downloading %s:%s: %w", repoPath, version, err)
	}

	// Remove .git directory to save space (cache is read-only)
	_ = os.RemoveAll(filepath.Join(tmpPath, ".git"))

	// Publish: swing any old export aside, move the fresh one in, then drop the
	// old — the no-tree window is the single rename between them, and a publish
	// failure restores the old export.
	gcPath := cachePath + ".gc"
	_ = os.RemoveAll(gcPath)
	hadOld := false
	if _, err := os.Stat(cachePath); err == nil {
		hadOld = true
		if err := os.Rename(cachePath, gcPath); err != nil {
			_ = os.RemoveAll(tmpPath)
			return "", fmt.Errorf("parking old repo cache %s: %w", cachePath, err)
		}
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		if hadOld {
			_ = os.Rename(gcPath, cachePath)
		}
		_ = os.RemoveAll(tmpPath)
		return "", fmt.Errorf("publishing repo cache %s: %w", cachePath, err)
	}
	_ = os.RemoveAll(gcPath)

	return cachePath, writeRefProvenance(cachePath, commit)
}

// GitDefaultBranch detects the default branch of a remote repository.
// Uses git ls-remote --symref to find what HEAD points to.
// Returns the branch name (e.g., "main", "master").
//
// This is the RAW primitive — it always hits the network. Caching is the
// GitClient's job (refs/git_client.go): every consumer that needs a cached answer
// goes through GitClient.DefaultBranch. The former per-primitive cache
// (latest-tags.json) is DELETED — one cache, one layer (R3/R5).
func GitDefaultBranch(repoURL string) (string, error) {
	out, err := runGitOp(false, "", nil, "ls-remote", "--symref", repoURL, "HEAD")
	if err != nil {
		return "", fmt.Errorf("git ls-remote --symref %s HEAD: %w", repoURL, err)
	}
	branch := parseDefaultBranch(string(out))
	if branch == "" {
		return "", fmt.Errorf("could not determine default branch for %s", repoURL)
	}
	return branch, nil
}

// parseDefaultBranch extracts the branch name from git ls-remote --symref output.
// Example line: "ref: refs/heads/main\tHEAD"
func parseDefaultBranch(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			// "ref: refs/heads/main\tHEAD" -> "main"
			ref := after
			if before, _, ok := strings.Cut(ref, "\t"); ok {
				return before
			}
		}
	}
	return ""
}

// GitLatestTag queries a remote repo for tags and returns the highest semver tag.
// Looks for tags matching v* pattern, sorts by semver, returns the highest.
// Returns an error if no version tags are found.
//
// This is the RAW primitive — it always hits the network. Caching is the
// GitClient's job (refs/git_client.go): every consumer that needs a cached answer
// goes through GitClient.LatestTag, which wraps this primitive with the persistent
// `cache:` section of the per-host charly.yml. The former per-primitive cache
// (latest-tags.json) is DELETED — one cache, one layer (R3/R5).
func GitLatestTag(repoURL string) (string, error) {
	out, err := runGitOp(false, "", nil, "ls-remote", "--tags", repoURL)
	if err != nil {
		return "", fmt.Errorf("git ls-remote --tags %s: %w", repoURL, err)
	}

	tags := parseTagRefs(string(out))
	if len(tags) == 0 {
		return "", fmt.Errorf("no version tags found in %s", repoURL)
	}

	sort.Slice(tags, func(i, j int) bool {
		return CompareSemver(tags[i], tags[j]) < 0
	})

	return tags[len(tags)-1], nil
}

// parseTagRefs extracts tag names from git ls-remote --tags output.
// Filters for v* tags and excludes peeled refs (^{}).
func parseTagRefs(output string) []string {
	var tags []string
	seen := make(map[string]bool)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ref := parts[1]
		// Skip peeled refs
		if strings.HasSuffix(ref, "^{}") {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		if !strings.HasPrefix(tag, "v") {
			continue
		}
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// CompareSemver compares two semver-like version strings (e.g. "v1.2.3").
// Returns -1 if a < b, 0 if equal, 1 if a > b.
// It delegates to spec/calver.CompareCalVer — the single dotted-version comparator
// (which strips the "v" prefix and falls back to lexical comparison for
// non-numeric parts) — so semver and CalVer strings share ONE implementation.
func CompareSemver(a, b string) int {
	return calver.CompareCalVer(a, b)
}

// isHex returns true if s contains only hexadecimal characters
func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return len(s) > 0
}

// cache freshness: see latest_tag_cache.go
