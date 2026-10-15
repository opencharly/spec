package schema_test

// skill_source_test.go — the `.agents/skills` FILE-source contract (the skills relocation): once a
// skill's body lives in a file, its `skill:` node carries only ROUTING metadata (family/owner/
// triggers/category), and the owning candy declares the tree those files are read from with
// `skills:`. This file is the SCHEMA half; the generator half is candy/plugin-marketplace.
//
// Three assertions here FAIL on the pre-change schema and are the ones the leg's gate rests on:
// the compile-time assertion below (#Candy had no Skills field at all), TestSkill_AcceptsRouting-
// MetadataOnly (name, description and content were all required, so a routing-only node was a hard
// rejection) and TestCandy_AcceptsSkillsTrees (#Candy is closed, so the new key was a
// `field not allowed`). The rest are guards: the in-line form the live entities still carry must
// keep working, and "optional" must not become "unconstrained".

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/spec"
)

// The generated Go must carry #Candy.skills. A COMPILE-TIME assertion, deliberately: the field is
// generated from the CUE and never hand-transcribed, so its absence from the generated types is a
// build failure — the strongest form this wiring check can take, and one the repo's
// TestGenReproducible cannot give (reproducibility says the file matches its generator, not that a
// given field exists).
var _ = spec.Candy{Skills: []string{".agents/skills"}}

// unifyDefComplete validates a value against a named definition under CONCRETE validation — the arm
// that reports a MISSING TOP-LEVEL FIELD, which is exactly the post-move `skill:` node's shape.
//
// Measured on the pinned cue (v0.16.1), which is why this helper exists beside unifyDefFinal:
//
//	#Skill {family: "internals", owner: "x"}          final=<nil>  concrete=incomplete value =~"^[a-z]…"
//	#CandyService {… wait_for: {timeout: "30s"}}      final=field is required but not present  concrete=same
//
// A missing field inside a NESTED literal is a "field is required but not present" that Final()
// reports; a missing field at the TOP level of the unified struct is instead an "incomplete value"
// that only Concrete(true) reports. The routing-only node is the top-level case, so the final
// validator above cannot fail on it.
func unifyDefComplete(t *testing.T, def, value string) error {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(src)
	d := v.LookupPath(cue.ParsePath(def))
	if !d.Exists() {
		t.Fatalf("%s is not defined in the shipped schema", def)
	}
	u := d.Unify(ctx.CompileString(value))
	if u.Err() != nil {
		return u.Err()
	}
	return u.Validate(cue.Concrete(true))
}

// TestSkill_AcceptsRoutingMetadataOnly — the post-move shape: family + owner + triggers, with the
// markdown body AND its frontmatter description living at .agents/skills/<name>/SKILL.md.
func TestSkill_AcceptsRoutingMetadataOnly(t *testing.T) {
	const routingOnly = `{
	family:   "internals"
	owner:    "charly-internals"
	triggers: ["editing a skill entity", "the marketplace corpus"]
}`
	if err := unifyDefComplete(t, "#Skill", routingOnly); err != nil {
		t.Errorf("#Skill rejects a routing-only node whose body lives in .agents/skills:\n%v", err)
	}
}

// TestSkill_StillAcceptsTheInlineBody — the TRANSITIONAL form, and the one the live tree depends
// on: every skill entity in the org still carries its body in-line today, so the relaxation has to
// be strictly permissive. A regression here would break the corpus generator for all of them.
func TestSkill_StillAcceptsTheInlineBody(t *testing.T) {
	const inline = `{
	name:        "git-workflow"
	family:      "internals"
	owner:       "charly-internals"
	description: "branch-per-change, PR-only landing."
	content:     "# git-workflow\n\n…"
	triggers:    ["commit / push / open a PR"]
	references: [{name: "validator-and-calver", content: "…"}]
}`
	if err := unifyDefComplete(t, "#Skill", inline); err != nil {
		t.Errorf("#Skill rejects the in-line body form every live entity still uses:\n%v", err)
	}
}

// TestSkill_DescriptionStaysNonEmpty — "optional" must not mean "unconstrained": an authored empty
// description is still a typo (the frontmatter description is the public subtitle on
// opencharly.ai), so the !="" refinement has to survive the relaxation.
func TestSkill_DescriptionStaysNonEmpty(t *testing.T) {
	const empty = `{
	family:      "internals"
	owner:       "charly-internals"
	description: ""
}`
	if err := unifyDefComplete(t, "#Skill", empty); err == nil {
		t.Error(`#Skill accepted an empty description — the !="" refinement is gone`)
	}
}

// TestCandy_AcceptsSkillsTrees — the candy declares where its skills are read from. `.agents/skills`
// is the declared default, so this is the explicit form; a repo that keeps its skills elsewhere is
// the reason the key exists. #Candy is CLOSED, so this case is a `field not allowed` before the
// change.
func TestCandy_AcceptsSkillsTrees(t *testing.T) {
	const declared = `{
	description: "concept candy owning this family's skills"
	skills:      [".agents/skills"]
}`
	if err := unifyDefFinal(t, "#Candy", declared); err != nil {
		t.Errorf("#Candy rejects a declared skills tree:\n%v", err)
	}
}

// TestCandy_RejectsAnEmptySkillsEntry — a path element is a real path: the empty string would make
// the generator read the repo root and publish whatever it found there.
func TestCandy_RejectsAnEmptySkillsEntry(t *testing.T) {
	const emptyEntry = `{
	description: "concept candy owning this family's skills"
	skills:      [""]
}`
	if err := unifyDefFinal(t, "#Candy", emptyEntry); err == nil {
		t.Error(`#Candy accepted an empty skills entry — the !="" constraint is gone`)
	}
}
