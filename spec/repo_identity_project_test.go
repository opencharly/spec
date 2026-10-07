package spec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repo_identity_project_test.go — the identity a LOCAL import ref resolves to must name the
// PROJECT, not merely the enclosing repo.
//
// Before this fix RepoIdentity() returned the git `origin` identity of the ref's directory, which
// `git remote get-url origin` resolves by walking UP to the enclosing repository. A namespaced
// import of a SUBDIRECTORY project inside the same repo therefore inherited the ROOT's identity,
// and walkNamespace's repo-identity cycle-break resolved it as a REFERENCE mount to the root — a
// degenerate self-cycle. Pre-fix that crashed the materialized wire marshaller
// (`json: unsupported value: encountered a cycle via map[string]*spec.UnifiedFile`); post-fix it
// silently dropped the whole namespace.

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
