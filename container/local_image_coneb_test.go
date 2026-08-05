package container

// local_image_coneb_test.go — relocated from sdk/kit/local_image_test.go (#55 coneB build-render
// cone, Class A). The resolution family moved here (local_image_coneb.go); the tests exercise the
// same bodies, now in package container. The package-level var overrides (ListLocalImages) target
// container.ListLocalImages — the var container.ResolveLocalImageRef / container.ResolveShellImageRef
// READ — so the stubs take effect (the kit re-export vars are value-copies that no longer affect
// these bodies).

import (
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
