package refs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPickResolvedCommit guards the Bug-2 fix: an annotated tag must resolve to
// the underlying COMMIT (refs/tags/X^{}), never the tag OBJECT (refs/tags/X).
// Returning the tag object made a later `git clone --depth 1 --branch <tag>`
// emit git's "refs/tags/X <sha> is not a commit!" warning.
func TestPickResolvedCommit(t *testing.T) {
	const tagObj = "c85de9810981f6655e8f9a5d2307460c0456d780"
	const commit = "2d731456b0b8cfbe2e19b64de75b4d652d2fc94c"
	cases := []struct {
		name      string
		lines     []string
		ref, want string
	}{
		{"annotated tag prefers the peeled commit, not the tag object",
			[]string{tagObj + "\trefs/tags/v1.0.0", commit + "\trefs/tags/v1.0.0^{}"}, "v1.0.0", commit},
		{"peeled line first still wins",
			[]string{commit + "\trefs/tags/v1.0.0^{}", tagObj + "\trefs/tags/v1.0.0"}, "v1.0.0", commit},
		{"lightweight tag (no peel) returns its direct sha",
			[]string{commit + "\trefs/tags/v1.0.0"}, "v1.0.0", commit},
		{"branch returns the head sha",
			[]string{commit + "\trefs/heads/main"}, "main", commit},
		{"ref absent returns empty",
			[]string{commit + "\trefs/tags/v9.9.9"}, "v1.0.0", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickResolvedCommit(c.lines, c.ref); got != c.want {
				t.Errorf("pickResolvedCommit(%v, %q) = %q, want %q", c.lines, c.ref, got, c.want)
			}
		})
	}
}

func TestParseDefaultBranch(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{"ref: refs/heads/main\tHEAD\nabc123\tHEAD\n", "main"},
		{"ref: refs/heads/master\tHEAD\ndef456\tHEAD\n", "master"},
		{"ref: refs/heads/develop\tHEAD\n789abc\tHEAD\n", "develop"},
		{"abc123\tHEAD\n", ""}, // no symref line
		{"", ""},               // empty output
	}

	for _, tt := range tests {
		got := parseDefaultBranch(tt.output)
		if got != tt.want {
			t.Errorf("parseDefaultBranch(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

func TestParseTagRefs(t *testing.T) {
	output := `abc123def456	refs/tags/v0.1.0
def456abc789	refs/tags/v0.1.0^{}
111222333444	refs/tags/v1.0.0
555666777888	refs/tags/v1.0.0^{}
aaa111bbb222	refs/tags/v2.0.0
ccc333ddd444	refs/tags/v2.0.0^{}
eee555fff666	refs/tags/not-semver
`
	tags := parseTagRefs(output)
	if len(tags) != 3 {
		t.Fatalf("len(tags) = %d, want 3", len(tags))
	}
	// Should contain v0.1.0, v1.0.0, v2.0.0 (no ^{} or non-v tags)
	want := map[string]bool{"v0.1.0": true, "v1.0.0": true, "v2.0.0": true}
	for _, tag := range tags {
		if !want[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v2.0.0", -1},
		{"v2.0.0", "v1.0.0", 1},
		{"v1.0.0", "v1.1.0", -1},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.9.0", "v1.10.0", -1},
		{"v0.1.0", "v1.0.0", -1},
	}

	for _, tt := range tests {
		got := CompareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsHex(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"abc123", true},
		{"ABC123", true},
		{"deadbeef", true},
		{"", false},
		{"xyz", false},
		{"abc 123", false},
	}

	for _, tt := range tests {
		got := isHex(tt.s)
		if got != tt.want {
			t.Errorf("isHex(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestRepoGitURL(t *testing.T) {
	got := RepoGitURL("github.com/opencharly/ml-layers")
	want := "https://github.com/opencharly/ml-layers.git"
	if got != want {
		t.Errorf("RepoGitURL() = %q, want %q", got, want)
	}
}

// TestPopulateSubmodules_NoGitmodulesIsNoOp: a repo declaring no submodules at
// all is a clean no-op — a box/<distro> repo has none and must not error.
//
// Note the deliberately narrowed contract: this used to also assert that a
// .gitmodules declaring only box/arch was a no-op, because just `sdk` was ever
// initialized. Populating EVERY declared submodule is the fix, so that case is
// no longer a no-op and asserting it would pin the bug in place.
func TestPopulateSubmodules_NoGitmodulesIsNoOp(t *testing.T) {
	if err := populateSubmodules(t.TempDir()); err != nil { // no .gitmodules at all
		t.Fatalf("no-.gitmodules dir must be a no-op, got %v", err)
	}
}

// TestGitClone_PopulatesAllSubmodules is the end-to-end integration gate: a
// fresh GitClone of the charly repo populates EVERY declared submodule, so a
// --repo cache is a project a user can actually drive with only the binary.
//
// This is the coverage that fails without the fix. `sdk` alone passed before
// and would still pass, so it proves nothing on its own — `spec` is the
// assertion that breaks against the old sdk-only special case, and it is the
// one that matters most: every out-of-process plugin candy carries
// `replace github.com/opencharly/spec => ../../spec`, so without it every such
// build dies on "../../spec/go.mod: no such file" and no out-of-process verb is
// reachable through --repo at all. box/<distro> covers `box build` (main owns
// no boxes), which the old code excluded as "heavy".
//
// Network-gated: skipped offline.
func TestGitClone_PopulatesAllSubmodules(t *testing.T) {
	if testing.Short() {
		t.Skip("network integration test")
	}
	if exec.Command("git", "ls-remote", "https://github.com/opencharly/charly.git", "HEAD").Run() != nil {
		t.Skip("no network / github unreachable")
	}
	dir := filepath.Join(t.TempDir(), "clone")
	if err := GitClone("https://github.com/opencharly/charly.git", "main", "", dir); err != nil {
		t.Fatalf("GitClone: %v", err)
	}
	for _, probe := range []struct{ path, why string }{
		{"sdk/go.mod", "plugin builds fail 'cannot load module ../../sdk'"},
		{"spec/go.mod", "plugin builds fail 'cannot load module ../../spec' — every out-of-process verb unreachable via --repo"},
		{"box/fedora/charly.yml", "`charly --repo … box build` has no box definitions; main owns none"},
		{"plugins/README.md", "the skill corpus is absent"},
	} {
		if _, err := os.Stat(filepath.Join(dir, probe.path)); err != nil {
			t.Errorf("submodule path %s not populated (%s): %v", probe.path, probe.why, err)
		}
	}
}
