package refs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git_files_test.go — the FILES-ONLY fetch (spec#279).
//
// The defect these tests pin: DownloadRepo populated EVERY submodule a fetched
// repo declares and failed the whole fetch when one was unreachable, so a repo
// with submodules (opencharly/opencharly declares 412) could not be a CORPUS
// SOURCE at all — even though the corpus reader reads only charly.yml and the
// .agents/ trees. The three arms are the issue's own acceptance criterion, each
// proven here against a LOCAL file:// remote (no network):
//
//	1. files-only fetch of a submodule-bearing repo SUCCEEDS and fetches none;
//	2. the whole-project fetch of the SAME repo still POPULATES (not a blanket opt-out);
//	3. a genuinely unreachable submodule still fails the whole-project fetch LOUDLY,
//	   while the files-only fetch of that very repo succeeds.
//
// Each test fails without the change: arm 1 and 3 do not compile before
// DownloadRepoFiles exists, and arm 2's assertion on the .files entry and arm 4's
// mode-relative freshness fail on the old predicate.

// gitTestRig runs git in a directory with a deterministic identity, failing the
// test on error. file protocol is enabled for SUBMODULE clones here because the
// remotes are local: git>=2.38 blocks `file://` for submodule transport by
// default (CVE-2022-39253), and the code under test builds its own git argv, so
// the allowance has to arrive through the environment.
type gitTestRig struct {
	t   *testing.T
	env []string
}

func newGitTestRig(t *testing.T) *gitTestRig {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Hermetic: the persistent submodule verdict cache must never touch the
	// operator's real cache dir.
	t.Setenv("CHARLY_CACHE_DIR", t.TempDir())
	// The code under test builds its OWN git argv, so the file-protocol allowance
	// has to reach it through the ENVIRONMENT (git's GIT_CONFIG_* triples), not
	// through -c on the rig's commands.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
	return &gitTestRig{t: t, env: append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_TERMINAL_PROMPT=0",
	)}
}

func (g *gitTestRig) run(dir string, args ...string) string {
	g.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = g.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		g.t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// mkSubRemote creates a bare remote holding one commit with one file.
func (g *gitTestRig) mkSubRemote(root, name, file string) string {
	g.t.Helper()
	remote := filepath.Join(root, name+".git")
	work := filepath.Join(root, name+"-work")
	g.run(root, "init", "--bare", "-q", "-b", "main", remote)
	g.run(root, "init", "-q", "-b", "main", work)
	if err := os.WriteFile(filepath.Join(work, file), []byte(name+"\n"), 0o644); err != nil {
		g.t.Fatal(err)
	}
	g.run(work, "add", file)
	g.run(work, "commit", "-q", "-m", name)
	g.run(work, "remote", "add", "origin", remote)
	g.run(work, "push", "-q", "origin", "main")
	return "file://" + remote
}

// mkSuperRemote creates a bare remote for a repo that declares ONE submodule and
// records its gitlink. With a reachable subURL it uses `git submodule add`; with
// an unreachable one it records the gitlink by hand (no clone is attempted at
// authoring time), which is exactly the shape that kills a whole-project fetch.
func (g *gitTestRig) mkSuperRemote(root, name, subURL string, reachable bool) string {
	g.t.Helper()
	remote := filepath.Join(root, name+".git")
	work := filepath.Join(root, name+"-work")
	g.run(root, "init", "--bare", "-q", "-b", "main", remote)
	g.run(root, "init", "-q", "-b", "main", work)
	if err := os.WriteFile(filepath.Join(work, "marker.txt"), []byte(name+"\n"), 0o644); err != nil {
		g.t.Fatal(err)
	}
	g.run(work, "add", "marker.txt")
	g.run(work, "commit", "-q", "-m", "super")
	g.run(work, "remote", "add", "origin", remote)
	g.run(work, "push", "-q", "origin", "main")

	if reachable {
		g.run(work, "-c", "protocol.file.allow=always", "submodule", "add", "-q", subURL, "sub")
	} else {
		// A gitlink whose .gitmodules URL cannot be reached. The commit sha only
		// has to be a plausible gitlink target; the fetch fails at CLONE time.
		sub := strings.TrimSpace(g.run(work, "rev-parse", "HEAD"))
		g.run(work, "update-index", "--add", "--cacheinfo", "160000,"+sub+",sub")
		gm := "[submodule \"sub\"]\n\tpath = sub\n\turl = " + subURL + "\n"
		if err := os.WriteFile(filepath.Join(work, ".gitmodules"), []byte(gm), 0o644); err != nil {
			g.t.Fatal(err)
		}
		g.run(work, "add", ".gitmodules")
	}
	g.run(work, "commit", "-q", "-m", "declare submodule")
	g.run(work, "push", "-q", "origin", "main")
	return "file://" + remote
}

// entriesIn lists a fetched export's submodule directory; git materializes an
// EMPTY placeholder for every gitlink it checks out, and a files-only export
// leaves it empty.
func entriesIn(t *testing.T, dir string) int {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	return len(ents)
}

// TestDownloadRepoFiles_SkipsDeclaredSubmodules is ARM 1: a corpus-source fetch of
// a repo that declares a submodule succeeds, reads the tree, and fetches no
// submodule content into its own cache entry.
func TestDownloadRepoFiles_SkipsDeclaredSubmodules(t *testing.T) {
	g := newGitTestRig(t)
	root := t.TempDir()
	t.Setenv("CHARLY_REPO_CACHE", filepath.Join(root, "cache"))
	subURL := g.mkSubRemote(root, "subrepo", "sub.txt")
	superURL := g.mkSuperRemote(root, "superrepo", subURL, true)

	dir, err := downloadRepoFromMode(superURL, "local/super", "main", fetchFiles)
	if err != nil {
		t.Fatalf("files-only fetch of a submodule-bearing repo must succeed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "marker.txt")); err != nil || !strings.Contains(string(got), "superrepo") {
		t.Fatalf("files-only export must carry the repo's own tree (marker.txt) — got %q, %v", got, err)
	}
	if n := entriesIn(t, filepath.Join(dir, "sub")); n != 0 {
		t.Fatalf("files-only export must populate NO submodule, but sub/ holds %d entries", n)
	}
	// The files-only entry is its own cache KEY (a mode directory), and its
	// BASENAME stays <repo>@<version> — the shape a consumer reads the source tag
	// out of (plugin-marketplace's resolve.go ranks twins by the CalVer at the end
	// of Entity.SourceRoot, so a suffix there would blind it).
	rel, relErr := filepath.Rel(os.Getenv("CHARLY_REPO_CACHE"), dir)
	if relErr != nil || !strings.HasPrefix(rel, filesCacheDir+string(filepath.Separator)) {
		t.Fatalf("files-only export must publish under the %q cache subdirectory, got %q (rel %q)", filesCacheDir, dir, rel)
	}
	if base := filepath.Base(dir); base != "super@main" {
		t.Fatalf("files-only entry basename must stay <repo>@<version> (source-tag shape), got %q", base)
	}
	projectPath, err := RepoCachePath("local/super", "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectPath); !os.IsNotExist(err) {
		t.Fatalf("a files-only fetch must not write the whole-project entry %s", projectPath)
	}
}

// TestDownloadRepo_PopulatesSubmodules is ARM 2 — the other caller still works:
// the WHOLE-PROJECT fetch of the very same repo populates its submodule.
func TestDownloadRepo_PopulatesSubmodules(t *testing.T) {
	g := newGitTestRig(t)
	root := t.TempDir()
	t.Setenv("CHARLY_REPO_CACHE", filepath.Join(root, "cache"))
	subURL := g.mkSubRemote(root, "subrepo2", "sub.txt")
	superURL := g.mkSuperRemote(root, "superrepo2", subURL, true)

	dir, err := downloadRepoFromMode(superURL, "local/super2", "main", fetchProject)
	if err != nil {
		t.Fatalf("whole-project fetch must succeed: %v", err)
	}
	if filesRel, _ := filepath.Rel(os.Getenv("CHARLY_REPO_CACHE"), dir); strings.HasPrefix(filesRel, filesCacheDir+string(filepath.Separator)) {
		t.Fatalf("whole-project export must publish to <repo>@<version>, got %q", dir)
	}
	if n := entriesIn(t, filepath.Join(dir, "sub")); n == 0 {
		t.Fatal("whole-project fetch must POPULATE the declared submodule — the --repo cache depends on it")
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "sub.txt")); err != nil {
		t.Fatalf("the populated submodule must hold the submodule's own content: %v", err)
	}
}

// TestUnreachableSubmodule_ProjectFailsLoud_FilesSucceeds is ARM 3 in one test,
// because the CONTRAST is the point: an unreachable declared submodule still
// fails the whole-project fetch by design (a silently partial --repo cache is
// the defect populateSubmodules exists to prevent), while the files-only fetch of
// that exact repo — which needs no submodule — succeeds.
func TestUnreachableSubmodule_ProjectFailsLoud_FilesSucceeds(t *testing.T) {
	g := newGitTestRig(t)
	root := t.TempDir()
	t.Setenv("CHARLY_REPO_CACHE", filepath.Join(root, "cache"))
	missing := "file://" + filepath.Join(root, "does-not-exist")
	superURL := g.mkSuperRemote(root, "broken", missing, false)

	_, err := downloadRepoFromMode(superURL, "local/broken", "main", fetchProject)
	if err == nil {
		t.Fatal("a genuinely unreachable submodule MUST fail the whole-project fetch — the fetch must not become universally lenient")
	}
	if !strings.Contains(err.Error(), "populating submodules in") {
		t.Fatalf("the whole-project failure must name the submodule population step, got: %v", err)
	}

	dir, err := downloadRepoFromMode(superURL, "local/broken", "main", fetchFiles)
	if err != nil {
		t.Fatalf("the files-only fetch of the SAME repo must succeed (it reads no submodule): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker.txt")); err != nil {
		t.Fatalf("files-only export must carry the repo's own tree: %v", err)
	}
}

// TestDownloadRepoFiles_ReusesFilesCache pins the freshness half: a files-only
// export is COMPLETE the moment its own tree is there, so a second fetch of the
// same immutable ref must reuse it instead of re-cloning forever (the failure
// mode a mode-blind predicate would produce — submodulesPopulated is false for a
// tree export by design, so a shared predicate would never accept it).
func TestDownloadRepoFiles_ReusesFilesCache(t *testing.T) {
	g := newGitTestRig(t)
	root := t.TempDir()
	t.Setenv("CHARLY_REPO_CACHE", filepath.Join(root, "cache"))
	subURL := g.mkSubRemote(root, "subrepo3", "sub.txt")
	superURL := g.mkSuperRemote(root, "superrepo3", subURL, true)

	first, err := downloadRepoFromMode(superURL, "local/super3", "main", fetchFiles)
	if err != nil {
		t.Fatalf("initial files-only download: %v", err)
	}
	// A sentinel in the export survives ONLY a cache hit: a re-clone replaces the
	// whole directory.
	sentinel := filepath.Join(first, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := downloadRepoFromMode(superURL, "local/super3", "main", fetchFiles)
	if err != nil {
		t.Fatalf("second files-only download: %v", err)
	}
	if second != first {
		t.Fatalf("cache path changed between identical resolutions: %q vs %q", second, first)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal("a files-only export was re-cloned on an unchanged ref — its own tree is not being accepted as complete")
	}
}

// TestRepoCacheFreshMode_FilesAcceptsTreeExport pins the predicate itself: one
// cache dir holding a complete tree and an UNPOPULATED submodule is fresh for the
// files-only reader and stale for the whole-project one. Without the mode split
// the files-only fetch would re-clone on every call.
func TestRepoCacheFreshMode_FilesAcceptsTreeExport(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, filesCacheDir, "repo@v1.0.0")
	commit := "2d731456b0b8cfbe2e19b64de75b4d652d2fc94c"
	if err := os.MkdirAll(filepath.Join(cachePath, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "charly.yml"), []byte("x:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A declared submodule with an EMPTY placeholder directory: exactly the tree
	// a files-only export leaves behind.
	if err := os.WriteFile(filepath.Join(cachePath, ".gitmodules"),
		[]byte("[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.invalid/sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteRepoCacheProvenance(cachePath, commit); err != nil {
		t.Fatal(err)
	}

	if !repoCacheFreshMode(cachePath, commit, fetchFiles) {
		t.Fatal("a tree export must be FRESH for the files-only reader")
	}
	if repoCacheFreshMode(cachePath, commit, fetchProject) {
		t.Fatal("the same export must be STALE for the whole-project reader — its submodule is not populated")
	}
	// Mode-relative freshness must not weaken the provenance contract.
	if repoCacheFreshMode(cachePath, "aaa111bbb222aaa111bbb222aaa111bbb222aaa111", fetchFiles) {
		t.Fatal("a files-only entry naming a different commit must be stale")
	}
}
