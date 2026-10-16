package spec

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCandyModelCarriesOwnedSkillNames is the coverage for opencharly/spec#254: the wire
// #CandyModel is the schema's own declared BUILD model, and `skill.cue` says a skill's
// `owner:` is "the build-time association that decides which images carry it". So a build
// model that omits it cannot answer, on its own, which skills a candy carries.
//
// WHY THIS TEST AND NOT A REPRODUCIBILITY CHECK. TestGenReproducible compares the committed
// generated file against a fresh `charly task cue-gen`, so it passes with or without the
// field — both sides regenerate consistently. It proves the pipeline is reproducible, which
// is a different claim from "the model carries the association". This test fails without the
// schema change, which is what R7 requires of coverage.
//
// It asserts three things, in the order a consumer depends on them:
//
//  1. the Go field EXISTS (without it, a wire-only consumer has no way to ask the question);
//  2. its WIRE KEY is the authored snake_case name `skills` — the tag is what crosses the
//     process boundary, so a correct Go name with the wrong tag is still a broken contract;
//  3. it ROUND-TRIPS through YAML as a list of skill names, which is the shape the
//     association takes (`owner` is a candy name; the skill's identity is its node name).
func TestCandyModelCarriesOwnedSkillNames(t *testing.T) {
	sf, ok := reflect.TypeOf(CandyModel{}).FieldByName("Skills")
	if !ok {
		t.Fatal("spec.CandyModel has no Skills field: a wire-only consumer cannot see which skills a candy owns, " +
			"which is the wire-contract gap opencharly/spec#254 reports")
	}

	// The wire key crosses the boundary, so it is checked rather than assumed.
	if tag := sf.Tag.Get("yaml"); !strings.HasPrefix(tag, "skills,") && tag != "skills" {
		t.Fatalf("spec.CandyModel.Skills yaml tag = %q, want the authored wire key `skills`", tag)
	}
	if tag := sf.Tag.Get("json"); !strings.HasPrefix(tag, "skills,") && tag != "skills" {
		t.Fatalf("spec.CandyModel.Skills json tag = %q, want the authored wire key `skills`", tag)
	}

	// Owner-qualified NAMES, not opaque bodies: the bodies travel through Config.Skills and
	// the marketplace generator, so this field must not become a second source for content
	// the model does not resolve.
	if got, want := sf.Type, reflect.TypeOf([]string{}); got != want {
		t.Fatalf("spec.CandyModel.Skills type = %s, want %s (the skill node NAMES this candy owns)", got, want)
	}

	// And it survives the boundary in both directions.
	owned := []string{"git-workflow", "go"}
	raw, err := yaml.Marshal(CandyModel{Name: "charly-internals", Skills: owned})
	if err != nil {
		t.Fatalf("marshal CandyModel with skills: %v", err)
	}
	if !strings.Contains(string(raw), "skills:") {
		t.Fatalf("marshalled CandyModel has no `skills:` key:\n%s", raw)
	}

	var back CandyModel
	if err := yaml.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal CandyModel with skills: %v", err)
	}
	if len(back.Skills) != len(owned) {
		t.Fatalf("round-tripped skills = %v, want %v", back.Skills, owned)
	}
	for i := range owned {
		if back.Skills[i] != owned[i] {
			t.Fatalf("round-tripped skills = %v, want %v", back.Skills, owned)
		}
	}
}
