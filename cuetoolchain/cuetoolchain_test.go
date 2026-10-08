package cuetoolchain

import (
	"strings"
	"testing"
)

// TestPinnedArchIsCompleteAndVerifiable pins the contract a caller relies on: for
// the arch we ship, the version, the tarball, the URL and the checksum all exist
// and agree with each other. A pin that is missing any of the three would leave
// the caller fetching something it cannot verify.
func TestPinnedArchIsCompleteAndVerifiable(t *testing.T) {
	sum, ok := SHA256("amd64")
	if !ok {
		t.Fatal("amd64 is not checksum-pinned; the verb could not verify the toolchain")
	}
	if len(sum) != 64 {
		t.Errorf("sha256 for amd64 is %d chars, want 64", len(sum))
	}
	tb := Tarball("amd64")
	if tb != "cue_"+Version+"_linux_amd64.tar.gz" {
		t.Errorf("Tarball(amd64) = %q", tb)
	}
	u := URL("amd64")
	if !strings.HasPrefix(u, "https://github.com/cue-lang/cue/releases/download/"+Version+"/") {
		t.Errorf("URL(amd64) = %q does not point at the pinned release", u)
	}
	if !strings.HasSuffix(u, tb) {
		t.Errorf("URL(amd64) = %q does not end in the tarball name %q", u, tb)
	}
}

// TestUnpinnedArchIsReportedNotGuessed is the loud-failure half: an arch with no
// checksum must report "not pinned" and produce no URL, so the caller fails with
// a remedy instead of downloading a binary it cannot verify.
func TestUnpinnedArchIsReportedNotGuessed(t *testing.T) {
	for _, arch := range []string{"arm64", "386", "arm", "riscv64", ""} {
		if sum, ok := SHA256(arch); ok {
			t.Errorf("SHA256(%q) reports pinned (%q); only amd64 is checksum-pinned", arch, sum)
		}
		if u := URL(arch); u != "" {
			t.Errorf("URL(%q) = %q, want \"\" for an unpinned arch", arch, u)
		}
		if tb := Tarball(arch); tb != "" {
			t.Errorf("Tarball(%q) = %q, want \"\" for an unpinned arch", arch, tb)
		}
	}
}
