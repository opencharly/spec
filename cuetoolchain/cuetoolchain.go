// Package cuetoolchain is THE single home for the pinned `cue` CLI (R3).
//
// The schema→Go pipeline runs `cue exp gengotypes`, and that generator lives in
// `cuelang.org/go`'s INTERNAL tree (`internal/encoding/gotypes`), so no other
// module can call it in-process. The CLI is therefore a toolchain the charly-side
// generation verb provisions itself — checksum-verified, from a pinned release —
// so a reader who is told to run a `charly` command never sees `cue`, a checkout
// path, or a `cd`.
//
// The pin is declared HERE, once, so the two consumers cannot drift: the
// `charly`-side generation verb reads it to fetch and verify the toolchain, and
// spec's own `bootstrap-cue` task is re-keyed onto it (it used to carry the same
// version and checksum inline in shell). One pin, one home.
//
// STANDALONE: stdlib only.
package cuetoolchain

import "fmt"

// Version is the pinned release tag of the `cue` CLI.
const Version = "v0.16.1"

// Member is the path of the CLI inside the release tarball.
const Member = "cue"

// sha256ByArch pins the release tarball per GOARCH. Only the arches listed are
// usable: a caller must consult SHA256 and fail LOUDLY on an unpinned arch rather
// than download a binary it cannot verify. Widening this map is a deliberate act
// (a checksum someone verified), never a fallback.
var sha256ByArch = map[string]string{
	"amd64": "5d644c1305a2b86504c8dcd2ec829cf5b4999efc2cf51ee375624e0455f774ae",
}

// Tarball returns the release tarball name for a GOARCH, or "" when that arch is
// not checksum-pinned.
func Tarball(goarch string) string {
	if _, ok := sha256ByArch[goarch]; !ok {
		return ""
	}
	return fmt.Sprintf("cue_%s_linux_%s.tar.gz", Version, goarch)
}

// URL returns the pinned download URL for a GOARCH, or "" when that arch is not
// checksum-pinned.
func URL(goarch string) string {
	t := Tarball(goarch)
	if t == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/cue-lang/cue/releases/download/%s/%s", Version, t)
}

// SHA256 returns the pinned checksum for a GOARCH and whether that arch is
// pinned at all — the check a caller must make BEFORE fetching, so an unpinned
// arch fails with a remedy rather than an unverifiable download.
func SHA256(goarch string) (string, bool) {
	sum, ok := sha256ByArch[goarch]
	return sum, ok
}
