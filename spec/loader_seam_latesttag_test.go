package spec

import "testing"

// TestRefsCollectSeams_LatestTagSeam pins the LatestTag seam field (the candy de-submodule
// cutover, Phase 4): version-less remote refs resolve to the repo's latest tag via a
// host-supplied callback, so the loaderkit mechanism is unit-testable offline. This test
// COMPILES ONLY IF the field exists — with the pre-seam spec it fails at build time
// ("spec.RefsCollectSeams has no field or method LatestTag"), the failing-without-it proof.
func TestRefsCollectSeams_LatestTagSeam(t *testing.T) {
	seams := RefsCollectSeams{
		LatestTag: func(repoURL string) (string, error) { return "v2026.235.1653", nil },
	}
	if seams.LatestTag == nil {
		t.Fatal("LatestTag seam not wired")
	}
	tag, err := seams.LatestTag("https://github.com/opencharly/layer-ripgrep.git")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if tag != "v2026.235.1653" {
		t.Fatalf("LatestTag = %q; want the stub tag", tag)
	}
}
