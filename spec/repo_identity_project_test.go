package spec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repo_identity_project_test.go — an import ref's identity must name the PROJECT, not merely its
// enclosing repo, for BOTH ref forms.
//
// RepoIdentity() used to return the git `origin` identity of a local ref's directory, which
// `git remote get-url origin` resolves by walking UP to the enclosing repository. A namespaced
// import of a SUBDIRECTORY project inside the same repo therefore inherited the ROOT's identity,
// and walkNamespace's repo-identity cycle-break resolved it as a REFERENCE mount to the root — a
// degenerate self-cycle. A remote `@host/org/repo[/sub]` ref collapsed the same way, by dropping
// its sub-path.
//
// The consequences are covered by two DIFFERENT fixes, and this file pins the identity one:
//
//   - the WIRE consequence — a plain json.Marshal of the cyclic graph is rejected with
//     `json: unsupported value: encountered a cycle via map[string]*spec.UnifiedFile` — is fixed
//     by the loaderkit codec (opencharly/sdk#350), whose flat form TRUNCATES a back-edge. With the
//     identity collapse still present, that truncation silently dropped the whole namespace.
//   - the IDENTITY consequence, the subject HERE: each subdirectory project resolves as its OWN
//     project (a definition mount) on both the local and the remote ref form.

func gitInitWithOrigin(t *testing.T, dir, origin string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("remote", "add", "origin", origin)
}

// TestRepoIdentity_SubdirectoryProjectIsDistinct: a subdirectory project inside the repo must NOT
// share the root's identity — the exact false back-edge that made the loader resolve it to the
// root.
func TestRepoIdentity_SubdirectoryProjectIsDistinct(t *testing.T) {
	root := t.TempDir()
	gitInitWithOrigin(t, root, "https://github.com/opencharly/plugin-check.git")
	sub := filepath.Join(root, "testdata", "fixture")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	rootID := RootRepoIdentity(root)
	if rootID != "github.com/opencharly/plugin-check" {
		t.Fatalf("root identity = %q, want github.com/opencharly/plugin-check", rootID)
	}
	subID := RepoIdentity(filepath.Join("testdata", "fixture"), root)
	if subID == rootID {
		t.Fatalf("subdirectory project inherited the root identity (%q) — the false back-edge", subID)
	}
	if want := "github.com/opencharly/plugin-check/testdata/fixture"; subID != want {
		t.Fatalf("subdirectory identity = %q, want %q", subID, want)
	}
}

// TestRepoIdentity_RemoteSubPathIsProjectScoped: the REMOTE form must not drop its sub-path
// either — a remote sub-path addresses the subdirectory project, not the enclosing repo.
func TestRepoIdentity_RemoteSubPathIsProjectScoped(t *testing.T) {
	if got := RepoIdentity("@github.com/opencharly/plugin-check/testdata/fixture:main", "/tmp"); got != "github.com/opencharly/plugin-check/testdata/fixture" {
		t.Fatalf("remote sub-path identity = %q, want github.com/opencharly/plugin-check/testdata/fixture", got)
	}
	// A remote ref with NO sub-path still names the repo-level project, so a local repo-root mount
	// and a remote ref to the same repo keep sharing one identity.
	if got := RepoIdentity("@github.com/opencharly/charly:main", "/tmp"); got != "github.com/opencharly/charly" {
		t.Fatalf("remote repo-root identity = %q, want github.com/opencharly/charly", got)
	}
}

// TestRepoIdentity_ExplicitRepoIsAuthoritative: a project's own `repo:` declaration is
// authoritative, exactly as RootRepoIdentity has always treated the root's.
func TestRepoIdentity_ExplicitRepoIsAuthoritative(t *testing.T) {
	root := t.TempDir()
	gitInitWithOrigin(t, root, "https://github.com/opencharly/plugin-check.git")
	sub := filepath.Join(root, "fixture")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, UnifiedFileName), []byte("repo: github.com/opencharly/dotted-fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RepoIdentity("fixture", root); got != "github.com/opencharly/dotted-fixture" {
		t.Fatalf("explicit repo identity = %q, want github.com/opencharly/dotted-fixture", got)
	}
}

// TestRepoIdentity_RepoRootKeepsBareIdentity: a ref that addresses a repo ROOT (the local
// submodule case) keeps the bare repo identity, so it still matches a remote ref to the SAME repo
// — the local<->remote mutual-import short-circuit must not regress.
func TestRepoIdentity_RepoRootKeepsBareIdentity(t *testing.T) {
	root := t.TempDir()
	gitInitWithOrigin(t, root, "git@github.com:opencharly/charly.git")
	if got := RepoIdentity(".", root); got != "github.com/opencharly/charly" {
		t.Fatalf("repo-root local identity = %q, want github.com/opencharly/charly", got)
	}
}

// TestRepoIdentity_LocalRefCannotEscapeItsRepository is the CONTAINMENT arm (charly#848): a local
// ref that resolves outside the importing project's git working tree must NOT name the project it
// finds there.
//
// Before this, the local ref path derived a directory from `ref` and handed it to
// ProjectRepoIdentity, which reads THAT directory's charly.yml — so a `repo:` declared by a
// neighbouring checkout was returned verbatim, from a read outside the project's own repository.
// The qualification path in the same file was already bounded; this arm pins that the ref path now
// obeys the same bound.
//
// The CONTROL arm matters as much as the escaping one: containment must refuse an ESCAPE, not
// resolve to "" for everything — a blanket refusal would satisfy the escaping assertion while
// destroying the identity the mutual-import cycle-break depends on.
func TestRepoIdentity_LocalRefCannotEscapeItsRepository(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	other := filepath.Join(base, "other-checkout")
	for _, d := range []string{repo, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitInitWithOrigin(t, repo, "https://github.com/opencharly/inside.git")
	gitInitWithOrigin(t, other, "https://github.com/opencharly/outside.git")
	// A DECLARED identity in the neighbour, so a cross-checkout read is visible in the result
	// rather than inferable: pre-change this exact string came back from the escaping ref.
	if err := os.WriteFile(filepath.Join(other, UnifiedFileName), []byte("repo: github.com/opencharly/outside-declared\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// CONTROL — a contained ref still resolves to its own project.
	if got := RepoIdentity(".", repo); got != "github.com/opencharly/inside" {
		t.Fatalf("contained repo-root ref = %q, want github.com/opencharly/inside (containment must not refuse everything)", got)
	}
	// CONTROL — a subdirectory project inside the repository is a definition mount, not an escape.
	sub := filepath.Join(repo, "testdata", "fixture")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepoIdentity(filepath.Join("testdata", "fixture"), repo); got != "github.com/opencharly/inside/testdata/fixture" {
		t.Fatalf("contained subdirectory ref = %q, want github.com/opencharly/inside/testdata/fixture", got)
	}

	// THE ARM — a ref that leaves the repository names no project in it.
	if got := RepoIdentity("../other-checkout", repo); got != "" {
		t.Fatalf("a ref escaping the repository named the other checkout: RepoIdentity(%q, repo) = %q, want \"\"", "../other-checkout", got)
	}
	// The ABSOLUTE spelling of the same escape must be refused identically — the bound is on where
	// the ref RESOLVES, not on how it was spelled.
	if got := RepoIdentity(other, repo); got != "" {
		t.Fatalf("an absolute escaping ref named the other checkout: RepoIdentity(%q, repo) = %q, want \"\"", other, got)
	}
}
