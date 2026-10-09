package spec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// repo_identity.go — repo identity for the import-namespace cycle-break, and --repo spec
// normalization. Pure fs/git/yaml logic with no *Config/*Candy dependency, relocated to the
// dedicated spec module (#55 2b Class A) so charly core + the loader-consuming plugins reach it
// without importing loaderkit. Consumed by the WalkProject entry point (candy/plugin-loader defaults
// the WalkSeams.RepoIdentity + rootIdentity) and candy/plugin-build's superproject resolve. charly's
// own loader-cone consumers (refs.go, main_repo.go) are GONE — K-wave 2 cone R1 moved the fetch
// orchestration and the --repo resolve into candy/plugin-loader. loaderkit re-exports these as
// forwarders for its own callers.
//
// The namespaced-import loader breaks mutual-import cycles by REPO IDENTITY (the `host/owner/repo`
// path), NOT by the pinned `:version`. This makes the importing project's namespace pins
// authoritative: when a transitively-imported release of some repo imports THIS repo back (the
// intentional main <-> cachyos mutual import) at a DIFFERENT pinned version, the back-reference
// resolves to the node already in progress up the load stack instead of fetching a divergent — and
// possibly stale-schema — snapshot.

// NormalizeRepoSpec turns a user-supplied --repo spec into a (repoPath, version) pair suitable for
// a repo-cache download. Spec formats:
//
//	"default"               → (DefaultProjectRepo, "")
//	"owner/repo"            → ("github.com/owner/repo", "")
//	"owner/repo@ref"        → ("github.com/owner/repo", "ref")
//	"host/owner/repo[@ref]" → used literally
//
// An empty version means "resolve to default branch at lookup time".
func NormalizeRepoSpec(spc string) (repoPath, version string) {
	spc = strings.TrimSpace(spc)
	if spc == "default" {
		return DefaultProjectRepo, ""
	}
	if before, after, ok := strings.Cut(spc, "@"); ok {
		repoPath, version = before, after
	} else {
		repoPath = spc
	}
	// Bare owner/repo (exactly one slash, no dots in the first segment) → auto-prefix github.com.
	// The dot-check distinguishes "github.com/foo" (already host-qualified) from "owner/repo".
	if slashes := strings.Count(repoPath, "/"); slashes == 1 {
		first, _, _ := strings.Cut(repoPath, "/")
		if !strings.Contains(first, ".") {
			repoPath = "github.com/" + repoPath
		}
	}
	return repoPath, version
}

// RepoIdentity returns the canonical identity of the PROJECT an import ref addresses, or "" when
// it can't be determined (in which case the loader degrades to version/path-keyed behavior). Both
// ref forms name the PROJECT, never its enclosing repo: a remote `@host/org/repo[/sub][:ver]` ref
// yields `host/org/repo` plus `/sub` when it carries a sub-path (no fetch, no git); a local path
// yields ProjectRepoIdentity of the target directory.
//
// The project — not the enclosing repo — is the unit the loader's cycle-break must key on:
// `git remote get-url origin` walks UP to the enclosing repository, so a subdirectory project
// inside one repo would otherwise inherit the ROOT's identity, and walkNamespace's repo-identity
// cycle-break would resolve that distinct project as a back-reference to the root — a degenerate
// self-cycle. The same collapse applies to a remote ref that names a sub-path.
func RepoIdentity(ref, baseDir string) string {
	if strings.HasPrefix(ref, "@") {
		if pr := ParseRemoteRef(ref); pr != nil {
			// A remote sub-path addresses a subdirectory PROJECT (the loader mounts the
			// charly.yml it contains), so its identity carries that path — mirroring the local
			// branch below, which qualifies by the path relative to the git toplevel.
			if sub := strings.Trim(pr.SubPath, "/"); sub != "" {
				return pr.RepoPath + "/" + sub
			}
			return pr.RepoPath
		}
		return ""
	}
	p := ref
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, ref)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	dir := abs
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	// CONTAINMENT — the ref path obeys the SAME bound the qualification path already obeys.
	//
	// Before this, a local ref resolved to ANY directory and ProjectRepoIdentity then read THAT
	// directory's charly.yml: `RepoIdentity("../other-checkout", dir)` returned the other
	// checkout's identity, and a `repo:` declared in its charly.yml was returned verbatim. The
	// project's own containment boundary is the one gitTopLevelRel already uses — the git WORKING
	// TREE TOPLEVEL of the importing project — so a sibling project inside the same repository
	// still resolves (that is a definition mount, not an escape), while a ref that leaves the
	// repository cannot name a project in it.
	//
	// An escaping ref is NOT an error: "" is this function's documented "cannot be determined"
	// answer — the same one it gives for a non-git directory — and the loader then degrades to
	// version/path-keyed identity, exactly as before. No new policy, and no new failure mode.
	if top := gitTopLevel(baseDir); top != "" {
		if _, ok := pathWithin(top, dir); !ok {
			return ""
		}
	}
	return ProjectRepoIdentity(dir)
}

// RootRepoIdentity determines the local root project's own identity for cycle-break registration
// — the SAME ProjectRepoIdentity a namespaced import of that project resolves to, so a transitive
// self-import matches the root seed exactly. Returns "" when it cannot be determined (the loader
// then behaves as before — version/path-keyed, no self-identity short-circuit).
func RootRepoIdentity(dir string) string {
	return ProjectRepoIdentity(dir)
}

// ProjectRepoIdentity returns the canonical identity of the PROJECT rooted at dir.
//
// An explicit `repo:` field in dir's charly.yml is authoritative (the rule RootRepoIdentity has
// always applied to the root, now applied uniformly). Otherwise it is the git `origin` identity
// of dir, QUALIFIED by the project's path relative to the git toplevel whenever dir is not itself
// the repo root — so a subdirectory project is distinct from its enclosing repo, while a
// repo-root project (a local submodule) keeps the bare identity and still matches a remote ref to
// the SAME repo.
func ProjectRepoIdentity(dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, UnifiedFileName)); err == nil {
		var head struct {
			Repo string `yaml:"repo" json:"repo"`
		}
		if yaml.Unmarshal(data, &head) == nil && head.Repo != "" {
			return NormalizeRepoIdentity(head.Repo)
		}
	}
	origin := GitRemoteIdentity(dir)
	if origin == "" {
		return ""
	}
	if rel := gitTopLevelRel(dir); rel != "" {
		return origin + "/" + rel
	}
	return origin
}

// gitTopLevelCache caches, per directory, dir's slash-separated path relative to its git TOPLEVEL
// — "" when dir IS the toplevel (or is not inside a git repo). Same stability argument as
// gitRemoteIdentityCache: a directory's toplevel does not change during a process run.
var gitTopLevelCache sync.Map // dir -> rel ("" = repo root / not a git repo)

// gitTopCache caches, per directory, the REAL (symlink-resolved) path of its git working-tree
// toplevel — "" when dir is not inside a git repo. Same stability argument as the caches around it.
var gitTopCache sync.Map // dir -> toplevel real path

// gitTopLevel returns the real path of dir's git working-tree toplevel, or "" when dir is not
// inside a git repo. This is the containment BOUNDARY both callers below share: GitRemoteIdentity
// walks UP to the repository, so the repository is the unit a ref may or may not stay inside.
func gitTopLevel(dir string) string {
	if v, ok := gitTopCache.Load(dir); ok {
		return v.(string)
	}
	top := ""
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		if raw := strings.TrimSpace(string(out)); raw != "" {
			if real, rerr := filepath.EvalSymlinks(raw); rerr == nil {
				top = real
			}
		}
	}
	gitTopCache.Store(dir, top)
	return top
}

// pathWithin reports dir's slash-separated path relative to top, and whether dir is contained in
// top AT ALL. It is the ONE implementation of the containment question, because two callers in this
// file need exactly it: gitTopLevelRel (to qualify an identity by a project's sub-path) and
// RepoIdentity (to refuse a local ref that escapes the project's repository). Both sides are
// resolved through symlinks first, so a symlinked route to the same place is the same place — and
// the guarded filepath.Rel is the predicate: "." (or "") means dir IS top, hence contained with no
// sub-path to add; a ".."-prefixed result means it is not below top at all.
func pathWithin(top, dir string) (string, bool) {
	dirReal, derr := filepath.EvalSymlinks(dir)
	if derr != nil {
		return "", false
	}
	r, rerr := filepath.Rel(top, dirReal)
	if rerr != nil {
		return "", false
	}
	if r == "." || r == "" {
		return "", true
	}
	if strings.HasPrefix(r, "..") {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// gitTopLevelRel reports dir's path relative to its git working-tree toplevel, or "" when dir is
// the toplevel itself or is not inside a git repo.
func gitTopLevelRel(dir string) string {
	if v, ok := gitTopLevelCache.Load(dir); ok {
		return v.(string)
	}
	rel := ""
	if top := gitTopLevel(dir); top != "" {
		if r, ok := pathWithin(top, dir); ok {
			rel = r
		}
	}
	gitTopLevelCache.Store(dir, rel)
	return rel
}

// gitRemoteIdentityCache caches the git `origin` identity per directory. The
// identity of a dir's git remote does NOT change during a process run (the
// process never modifies git remotes), so a process-wide cache is safe — and it
// eliminates the repeated `git remote get-url origin` subprocess spawns that
// dominated `charly status` (514 spawns on the per-host config dir, measured).
var gitRemoteIdentityCache sync.Map // dir -> identity ("" for non-git dirs)

// GitRemoteIdentity returns the normalized `host/owner/repo` identity of dir's git `origin`
// remote, or "" when dir is not a git repo / has no origin / git is unavailable. Cached
// process-wide per directory (the identity is stable for the process lifetime).
func GitRemoteIdentity(dir string) string {
	if v, ok := gitRemoteIdentityCache.Load(dir); ok {
		return v.(string)
	}
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	identity := ""
	if err == nil {
		identity = NormalizeGitRemoteURL(strings.TrimSpace(string(out)))
	}
	gitRemoteIdentityCache.Store(dir, identity)
	return identity
}

// NormalizeRepoIdentity normalizes an explicit `repo:` value (which may be a full git URL, an
// scp-style ref, or a bare `owner/repo`) to the `host/owner/repo` form ParseRemoteRef produces —
// so an explicit declaration and a remote `@`-ref compare equal. Reuses NormalizeRepoSpec's
// bare-`owner/repo` → github.com rule.
func NormalizeRepoIdentity(s string) string {
	repoPath, _ := NormalizeRepoSpec(NormalizeGitRemoteURL(s))
	return strings.TrimSuffix(repoPath, ".git")
}

// NormalizeGitRemoteURL strips the scheme / `git@` / `.git` decorations from a git remote URL,
// leaving `host/owner/repo`. scp-style (`git@host:owner/repo`) and scheme URLs (`https://`,
// `ssh://`, `git://`, with optional `user@`) are both handled. A value already in `host/owner/repo`
// (or bare `owner/repo`) form passes through unchanged.
func NormalizeGitRemoteURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	if after, ok := strings.CutPrefix(s, "git@"); ok {
		s = after
		return strings.Replace(s, ":", "/", 1)
	}
	for _, sch := range []string{"https://", "http://", "ssh://", "git://"} {
		if after, ok := strings.CutPrefix(s, sch); ok {
			s = after
			if slash := strings.Index(s, "/"); slash >= 0 {
				if at := strings.Index(s[:slash], "@"); at >= 0 {
					s = s[at+1:]
				}
			}
			return s
		}
	}
	return s
}
