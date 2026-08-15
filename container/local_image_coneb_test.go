package container

// local_image_coneb_test.go — relocated from sdk/kit/local_image_test.go (#55 coneB build-render
// cone, Class A). The resolution family moved here (local_image_coneb.go); the tests exercise the
// same bodies, now in package container. The package-level var overrides (ListLocalImages) target
// container.ListLocalImages — the var container.ResolveLocalImageRef / container.ResolveShellImageRef
// READ — so the stubs take effect (the kit re-export vars are value-copies that no longer affect
// these bodies).

import (
	"errors"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestParseLocalImagesJSON_DedupByID covers the root fix for the keep_images
// over-removal bug: podman emits ONE ROW PER TAG (each row's Names already lists
// every tag on that id), so the parser must collapse rows to ONE entry per
// distinct image id with the tag refs merged — not N near-identical entries.
func TestParseLocalImagesJSON_DedupByID(t *testing.T) {
	// Two rows for one id (id "ccc", two tags), each row carrying BOTH tags in
	// Names — exactly podman's row-per-tag shape. Plus a distinct id "ddd".
	js := []byte(`[
		{"Id":"ccc","Names":["ghcr/check-pod:2026.150.0916","ghcr/check-pod:2026.150.0836"],"Labels":{"ai.opencharly.image":"check-pod","ai.opencharly.version":"2026.155.1801"}},
		{"Id":"ccc","Names":["ghcr/check-pod:2026.150.0916","ghcr/check-pod:2026.150.0836"],"Labels":{"ai.opencharly.image":"check-pod","ai.opencharly.version":"2026.155.1801"}},
		{"Id":"ddd","Names":["ghcr/other:2026.001.0001"],"Labels":{"ai.opencharly.image":"other"}}
	]`)
	imgs, err := ParseLocalImagesJSON(js)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("got %d entries, want 2 (one per distinct id): %+v", len(imgs), imgs)
	}
	// id ccc: the two duplicate rows collapse to one entry with BOTH tags
	// (deduped, not 4 copies), labels preserved.
	if imgs[0].ID != "ccc" || len(imgs[0].Names) != 2 {
		t.Fatalf("entry 0 = %+v, want id ccc with 2 merged tags", imgs[0])
	}
	if imgs[0].Labels["ai.opencharly.image"] != "check-pod" || imgs[0].Labels["ai.opencharly.version"] != "2026.155.1801" {
		t.Fatalf("entry 0 labels not preserved: %+v", imgs[0].Labels)
	}
	if imgs[1].ID != "ddd" || len(imgs[1].Names) != 1 {
		t.Fatalf("entry 1 = %+v, want id ddd with 1 tag", imgs[1])
	}
}

// TestParseLocalImagesJSON_DockerRepoTags covers the docker shape (RepoTags
// instead of Names) and that distinct untagged (empty-id) rows do NOT merge.
func TestParseLocalImagesJSON_DockerRepoTags(t *testing.T) {
	js := []byte(`[
		{"ID":"aaa","RepoTags":["ghcr/foo:2026.001.0001"],"Labels":{"ai.opencharly.image":"foo"}},
		{"Id":"","Names":["<none>:<none>"]},
		{"Id":"","Names":["<none>:<none>"]}
	]`)
	imgs, err := ParseLocalImagesJSON(js)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 1 foo (RepoTags) + 2 distinct empty-id rows kept separate = 3 entries.
	if len(imgs) != 3 {
		t.Fatalf("got %d entries, want 3 (docker RepoTags + 2 unmerged empty-id): %+v", len(imgs), imgs)
	}
	if imgs[0].ID != "aaa" || len(imgs[0].Names) != 1 || imgs[0].Names[0] != "ghcr/foo:2026.001.0001" {
		t.Fatalf("entry 0 = %+v, want id aaa with RepoTags ref", imgs[0])
	}
}

// TestShortNameMatchesRef — relocated from charly/checkrun_charly_verbs_test.go (it tests this
// package's unexported shortNameMatchesRef).
func TestShortNameMatchesRef(t *testing.T) {
	cases := []struct {
		fullRef string
		short   string
		want    bool
	}{
		{"ghcr.io/opencharly/jupyter:latest", "jupyter", true},
		{"ghcr.io/opencharly/jupyter", "jupyter", true}, // no tag
		{"localhost/jupyter:v2", "jupyter", true},
		{"jupyter:latest", "jupyter", true}, // no registry
		{"ghcr.io/opencharly/jupyter:latest", "filebrowser", false},
		{"ghcr.io/opencharly/something-jupyter:latest", "jupyter", false}, // not a trailing match
	}
	for _, tc := range cases {
		got := shortNameMatchesRef(tc.fullRef, tc.short)
		if got != tc.want {
			t.Errorf("shortNameMatchesRef(%q, %q) = %v, want %v", tc.fullRef, tc.short, got, tc.want)
		}
	}
}

func TestResolveLocalImageRefShortTaggedPinsExactBuild(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		return []LocalImageInfo{
			{ID: "old", Names: []string{"ghcr.io/opencharly/check-agent-box:check-agent-pod-2026.199.1646"}, Labels: map[string]string{spec.LabelBox: "check-agent-box", spec.LabelVersion: "2026.199.1330"}},
			{ID: "new", Names: []string{"ghcr.io/opencharly/check-agent-box:check-agent-pod-2026.199.1700"}, Labels: map[string]string{spec.LabelBox: "check-agent-box", spec.LabelVersion: "2026.199.1330"}},
		}, nil
	}
	got, err := ResolveLocalImageRef("podman", "check-agent-box:check-agent-pod-2026.199.1700")
	if err != nil {
		t.Fatalf("ResolveLocalImageRef(short:tag): %v", err)
	}
	if want := "ghcr.io/opencharly/check-agent-box:check-agent-pod-2026.199.1700"; got != want {
		t.Fatalf("ResolveLocalImageRef(short:tag) = %q, want %q", got, want)
	}
}

// ResolveShellImageRef branch coverage (sdk#68 review round — the helper shipped
// with none; these five cases FAIL without the function's branch logic).
// ListLocalImages is stubbed via its package-level var (same pattern as
// LocalImageExists/DetectGPU testability notes on the var itself).
func TestResolveShellImageRef_TagEmptyLocalHit(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		return []LocalImageInfo{{
			ID:    "sha256:aa",
			Names: []string{"localhost/jupyter:2026.190.1200", "localhost/jupyter:2026.191.0800"},
			Labels: map[string]string{
				spec.LabelBox:     "jupyter",
				spec.LabelVersion: "2026.185.0000",
			},
		}}, nil
	}
	got := ResolveShellImageRef("ghcr.io/opencharly", "jupyter", "")
	// Local CalVer resolution wins over the registry fallback; newest tag-CalVer picked.
	if got != "localhost/jupyter:2026.191.0800" {
		t.Errorf("tag-empty local-hit: got %q, want the newest local CalVer ref", got)
	}
}

func TestResolveShellImageRef_TagEmptyLocalMissWithRegistry(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) { return nil, nil }
	got := ResolveShellImageRef("ghcr.io/opencharly", "jupyter", "")
	if got != "ghcr.io/opencharly/jupyter" {
		t.Errorf("tag-empty local-miss + registry: got %q, want tagless registry/name", got)
	}
}

func TestResolveShellImageRef_TagEmptyLocalMissNoRegistry(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) { return nil, nil }
	got := ResolveShellImageRef("", "jupyter", "")
	if got != "jupyter" {
		t.Errorf("tag-empty local-miss no-registry: got %q, want bare short name", got)
	}
}

func TestResolveShellImageRef_TagSetWithRegistry(t *testing.T) {
	// tag-set paths must NOT consult local storage at all.
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		t.Fatal("tag-set branch must not list local images")
		return nil, nil
	}
	got := ResolveShellImageRef("ghcr.io/opencharly", "jupyter", "2026.198.0001")
	if got != "ghcr.io/opencharly/jupyter:2026.198.0001" {
		t.Errorf("tag-set + registry: got %q", got)
	}
}

func TestResolveShellImageRef_TagSetNoRegistry(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		t.Fatal("tag-set branch must not list local images")
		return nil, nil
	}
	got := ResolveShellImageRef("", "jupyter", "2026.198.0001")
	if got != "jupyter:2026.198.0001" {
		t.Errorf("tag-set no-registry: got %q", got)
	}
}

// TestResolveLocalImageRef_NeverReturnsSiblingDeployAlias is the regression witness for the
// cross-deployment image-crossing defect. The fixture is the REAL local store shape measured on
// the dev host (`charly box list tags check-pod`): several disposable beds share ONE base box, so
// every bed's `tagDeployAlias` alias (`<registry>/<deploy-name>:<calver>`) sits on an image
// carrying the BASE box's ai.opencharly.image label, and — because the label-CalVer is
// content-derived — they all share ONE label-CalVer. Pre-fix, the tag-CalVer tiebreak therefore
// elected whichever sibling bed was (re)deployed most recently: an untagged resolve of the BASE
// box name returned another deployment's image.
//
// The property: an untagged resolve of box B returns a ref NAMED B, never a sibling's alias.
func TestResolveLocalImageRef_NeverReturnsSiblingDeployAlias(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	const labelCV = "2026.209.1500" // one content-derived label-CalVer across the whole family
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		return []LocalImageInfo{
			{ID: "base", Names: []string{"ghcr.io/opencharly/check-pod:2026.216.2119"},
				Labels: map[string]string{spec.LabelBox: "check-pod", spec.LabelVersion: labelCV}},
			// Sibling beds' aliases — NEWER tag-CalVers, same inherited base label.
			{ID: "sib1", Names: []string{"ghcr.io/opencharly/check-preempt-arbiter-pod:2026.216.2124"},
				Labels: map[string]string{spec.LabelBox: "check-pod", spec.LabelVersion: labelCV}},
			{ID: "sib2", Names: []string{"ghcr.io/opencharly/check-stepkind-emit-pod:2026.216.2120"},
				Labels: map[string]string{spec.LabelBox: "check-pod", spec.LabelVersion: labelCV}},
		}, nil
	}
	got, err := ResolveLocalImageRef("podman", "check-pod")
	if err != nil {
		t.Fatalf("ResolveLocalImageRef(check-pod): %v", err)
	}
	if want := "ghcr.io/opencharly/check-pod:2026.216.2119"; got != want {
		t.Fatalf("untagged base resolve = %q, want %q (a sibling deployment's alias must never win)", got, want)
	}
	// The aliases stay resolvable by their OWN deploy name via the name fallback — the alias's
	// entire purpose (deploy-name-keyed `charly config`/`charly start`).
	got, err = ResolveLocalImageRef("podman", "check-stepkind-emit-pod")
	if err != nil {
		t.Fatalf("ResolveLocalImageRef(check-stepkind-emit-pod): %v", err)
	}
	if want := "ghcr.io/opencharly/check-stepkind-emit-pod:2026.216.2120"; got != want {
		t.Fatalf("deploy-name resolve = %q, want %q", got, want)
	}
}

// TestRefRepoName covers the two shapes the promoted-to-filter predicate now has to get right
// (it gates candidacy, not just tie ordering): a registry PORT must not read as a tag, and a
// digest must be stripped.
func TestRefRepoName(t *testing.T) {
	cases := []struct{ ref, want string }{
		{"ghcr.io/opencharly/jupyter:2026.216.2119", "jupyter"},
		{"ghcr.io/opencharly/jupyter", "jupyter"},
		{"localhost/jupyter:v2", "jupyter"},
		{"jupyter:latest", "jupyter"},
		{"jupyter", "jupyter"},
		{"localhost:5000/jupyter", "jupyter"},                             // registry port, no tag
		{"localhost:5000/jupyter:2026.216.2119", "jupyter"},               // registry port + tag
		{"ghcr.io/opencharly/jupyter@sha256:abc123", "jupyter"},           // digest
		{"ghcr.io/opencharly/versa/ecovoyage:2026.216.2119", "ecovoyage"}, // instance alias
	}
	for _, tc := range cases {
		if got := refRepoName(tc.ref); got != tc.want {
			t.Errorf("refRepoName(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}

// --- the stale-build-election guard (charly#check-box-target-image) ---

// staleReproStorage reproduces the MEASURED local storage of the incident (2026-08-15, day 227,
// host `podman images` capture) that made `charly check box fedora-nonfree` print
// `Image: ghcr.io/opencharly/fedora-nonfree:2026.216.1908` and report `5 passed, 0 failed`
// against a plan that predated the candy edit under test.
//
// The two families and why they split:
//   - the OLD images carry `ai.opencharly.box=fedora-nonfree` (built when the box was named
//     unqualified) — they form the LABEL family;
//   - the FRESH build carries `ai.opencharly.box=fedora.fedora-nonfree`, because the render used
//     to label with the Generator's namespace-qualified map key while tagging the ref with the
//     leaf name — so it lands in the NAME family;
//   - the election takes the label family whole and DISCARDS the name family, so the newest build
//     is invisible and the newest OLD tag (2026.216.1908) wins.
func staleReproStorage() []LocalImageInfo {
	return []LocalImageInfo{
		{ID: "old", Names: []string{
			"ghcr.io/opencharly/fedora-nonfree:2026.216.1516",
			"ghcr.io/opencharly/fedora-nonfree:2026.216.1908",
		}, Labels: map[string]string{spec.LabelBox: "fedora-nonfree", spec.LabelVersion: "2026.144.1443"}},
		{ID: "fresh", Names: []string{
			"ghcr.io/opencharly/fedora-nonfree:2026.227.0835",
			"ghcr.io/opencharly/fedora-nonfree:2026.227.0836",
		}, Labels: map[string]string{spec.LabelBox: "fedora.fedora-nonfree", spec.LabelVersion: "2026.227.0830"}},
	}
}

// TestResolveBuiltImageRef_RefusesStaleElection is the regression gate for the incident: a verb
// that pronounces a verdict on a built artifact must REFUSE rather than certify an image older
// than the newest local build. Fails without the guard (ResolveBuiltImageRef would return the
// 2026.216.1908 ref with a nil error, exactly as ResolveLocalImageRef still does below).
func TestResolveBuiltImageRef_RefusesStaleElection(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(string) ([]LocalImageInfo, error) { return staleReproStorage(), nil }

	got, err := ResolveBuiltImageRef("podman", "fedora-nonfree")
	if err == nil {
		t.Fatalf("ResolveBuiltImageRef(fedora-nonfree) = %q, nil — want a refusal: %s is a newer local build",
			got, "ghcr.io/opencharly/fedora-nonfree:2026.227.0836")
	}
	if !errors.Is(err, spec.ErrStaleLocalImage) {
		t.Fatalf("error %v does not wrap spec.ErrStaleLocalImage", err)
	}
	// The message must name BOTH refs — the one it would have certified and the one the operator
	// almost certainly meant — plus a runnable re-invocation. A refusal the operator cannot act
	// on is only marginally better than the silent pass it replaces.
	for _, want := range []string{
		"ghcr.io/opencharly/fedora-nonfree:2026.216.1908",
		"ghcr.io/opencharly/fedora-nonfree:2026.227.0836",
		"fedora-nonfree:2026.227.0836",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %q", err.Error(), want)
		}
	}
}

// TestResolveLocalImageRef_LenientFormStillElects pins the deliberate split: the lenient resolver
// (every consumption path — deploy, vm build, builder bootstrap) keeps its existing election and
// its existing ordering. The guard is a property of the VERDICT verbs, not a change to resolution.
func TestResolveLocalImageRef_LenientFormStillElects(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(string) ([]LocalImageInfo, error) { return staleReproStorage(), nil }

	got, err := ResolveLocalImageRef("podman", "fedora-nonfree")
	if err != nil {
		t.Fatalf("ResolveLocalImageRef(fedora-nonfree): %v", err)
	}
	if want := "ghcr.io/opencharly/fedora-nonfree:2026.216.1908"; got != want {
		t.Fatalf("lenient resolve = %q, want %q (unchanged election)", got, want)
	}
}

// TestResolveBuiltImageRef_PinnedInputPassesThrough covers the escape hatch AND the reason the R10
// bed sequence is untouched by the guard: the bed builds `<image> --tag <run-tag>` and then checks
// `<image>:<run-tag>`, an explicit pin, so the guard never fires there.
func TestResolveBuiltImageRef_PinnedInputPassesThrough(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(string) ([]LocalImageInfo, error) { return staleReproStorage(), nil }

	// An explicit tag on the OLDER image: the operator said which artifact they meant.
	got, err := ResolveBuiltImageRef("podman", "fedora-nonfree:2026.216.1908")
	if err != nil {
		t.Fatalf("ResolveBuiltImageRef(pinned older tag): %v", err)
	}
	if want := "ghcr.io/opencharly/fedora-nonfree:2026.216.1908"; got != want {
		t.Fatalf("pinned resolve = %q, want %q", got, want)
	}
	// And the newest build resolves by its own tag, which is what the refusal tells you to run.
	got, err = ResolveBuiltImageRef("podman", "fedora-nonfree:2026.227.0836")
	if err != nil {
		t.Fatalf("ResolveBuiltImageRef(pinned newest tag): %v", err)
	}
	if want := "ghcr.io/opencharly/fedora-nonfree:2026.227.0836"; got != want {
		t.Fatalf("pinned resolve = %q, want %q", got, want)
	}
}

// TestResolveBuiltImageRef_ConsistentLabelsElectNewestBuild proves the guard does NOT fire once
// the emitter labels every build with the box's LEAF name (the render fix in
// sdk/deploykit.buildBakedMetadata): one family, the fresh build's higher content-derived
// label-CalVer wins outright, and `charly check box fedora-nonfree` just works.
func TestResolveBuiltImageRef_ConsistentLabelsElectNewestBuild(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	ListLocalImages = func(string) ([]LocalImageInfo, error) {
		imgs := staleReproStorage()
		imgs[1].Labels[spec.LabelBox] = "fedora-nonfree" // what the fixed render emits
		return imgs, nil
	}

	got, err := ResolveBuiltImageRef("podman", "fedora-nonfree")
	if err != nil {
		t.Fatalf("ResolveBuiltImageRef(fedora-nonfree) with consistent labels: %v", err)
	}
	if want := "ghcr.io/opencharly/fedora-nonfree:2026.227.0836"; got != want {
		t.Fatalf("resolve = %q, want %q (the newest build)", got, want)
	}
}

// TestResolveBuiltImageRef_SiblingAliasIsNotANewerBuild guards the guard: a sibling deployment's
// `<deploy-name>:<calver>` alias must not read as "a newer build of this box". The alias is named
// for the OTHER deployment, so it is never a candidate for this short name and cannot trigger a
// refusal — otherwise every bed host would refuse every untagged verdict.
func TestResolveBuiltImageRef_SiblingAliasIsNotANewerBuild(t *testing.T) {
	orig := ListLocalImages
	defer func() { ListLocalImages = orig }()
	const labelCV = "2026.209.1500"
	ListLocalImages = func(string) ([]LocalImageInfo, error) {
		return []LocalImageInfo{
			{ID: "base", Names: []string{"ghcr.io/opencharly/check-pod:2026.216.2119"},
				Labels: map[string]string{spec.LabelBox: "check-pod", spec.LabelVersion: labelCV}},
			{ID: "sib", Names: []string{"ghcr.io/opencharly/check-preempt-arbiter-pod:2026.216.2124"},
				Labels: map[string]string{spec.LabelBox: "check-pod", spec.LabelVersion: labelCV}},
		}, nil
	}
	got, err := ResolveBuiltImageRef("podman", "check-pod")
	if err != nil {
		t.Fatalf("ResolveBuiltImageRef(check-pod): %v", err)
	}
	if want := "ghcr.io/opencharly/check-pod:2026.216.2119"; got != want {
		t.Fatalf("resolve = %q, want %q", got, want)
	}
}
