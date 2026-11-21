package schema_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// owned_content_test.go — the #OwnedContent* wire shapes of ops.OpContent (M1, Content -> IR): what a
// kind is asked about one box, and what its owned nodes contribute (opencharly/sdk#598).

// validateDef unifies a literal with one definition of the shipped schema and validates it concretely.
func validateDef(t *testing.T, def, literal string) error {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(src)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	d := v.LookupPath(cue.ParsePath(def))
	if !d.Exists() {
		t.Fatalf("%s is not defined in the shipped schema", def)
	}
	u := d.Unify(ctx.CompileString(literal))
	if u.Err() != nil {
		return u.Err()
	}
	return u.Validate(cue.Concrete(true))
}

// A kind's reply carries home-relative files and JSON-valued labels; the request names the box, the
// image user, the composed candies and the kind's raw nodes.
func TestOwnedContentShapesAcceptTheContract(t *testing.T) {
	cases := map[string]string{
		"#ContentFile":         `{source: "/cache/distro-arch@v1/.agents/skills/arch/SKILL.md", path: ".agents/skills/arch/SKILL.md", mode: 0o644}`,
		"#OwnedContentReply":   `{files: [{source: "/r/a", path: "a"}], labels: {"ai.opencharly.skill": [{name: "arch"}]}}`,
		"#OwnedContentRequest": `{box: "arch", home: "/home/user", uid: 1000, gid: 1000, candies: [{name: "arch-skills", repo_root: "/cache/distro-arch@v1"}], nodes: {"arch-skill": {owner: "arch-skills"}}}`,
		"#OwnedContentReply ":  `{diagnostics: ["skill \"x\": owner \"img\" is an image"]}`,
		// Project scope (validation): no box, so no image user is needed.
		"#OwnedContentRequest ": `{candies: [{name: "c", repo_root: "/r"}], images: ["img"], nodes: {}}`,
	}
	for key, lit := range cases {
		def := strings.TrimSpace(key) // a trailing space keys a second case of the same definition
		if err := validateDef(t, def, lit); err != nil {
			t.Errorf("%s rejects %s: %v", def, lit, err)
		}
	}
}

// Every constraint the render relies on rejects: a relative source would resolve against whatever
// directory the render runs in; an absolute or climbing target would let a kind write outside the
// image user's home; a mode beyond the permission bits is not a mode; a request without a home has
// nowhere to install; a candy's repo root must be absolute.
func TestOwnedContentShapesRejectWhatTheRenderCannotHonour(t *testing.T) {
	cases := []struct{ def, lit string }{
		{"#ContentFile", `{source: "rel/SKILL.md", path: ".agents/skills/a/SKILL.md"}`},
		{"#ContentFile", `{source: "/r/a", path: "/etc/passwd"}`},
		{"#ContentFile", `{source: "/r/a", path: "../.bashrc"}`},
		{"#ContentFile", `{source: "/r/a", path: ".agents/../../.bashrc"}`},
		{"#ContentFile", `{source: "/r/a", path: "a", mode: 0o10000}`},
		{"#ContentFile", `{source: "/r/a"}`},
		{"#OwnedContentRequest", `{box: "arch", uid: 1000, gid: 1000}`},
		{"#OwnedContentRequest", `{box: "arch", home: "/home/user", uid: 1000, gid: 1000, candies: [{name: "x", repo_root: "rel"}]}`},
	}
	for _, c := range cases {
		if err := validateDef(t, c.def, c.lit); err == nil {
			t.Errorf("%s accepts %s", c.def, c.lit)
		}
	}
}
