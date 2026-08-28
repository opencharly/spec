package schema_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// boxInit compiles the shipped schema and validates a #Box carrying the given init
// value, returning whether it satisfies the schema.
func boxInit(t *testing.T, init string) error {
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
	box := v.LookupPath(cue.ParsePath("#Box"))
	if !box.Exists() {
		t.Fatal("#Box is not defined in the shipped schema")
	}
	unified := box.Unify(ctx.CompileString(`{init: "` + init + `"}`))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(false))
}

// #Box.init has to accept exactly the init systems the build vocabulary defines.
// It listed only supervisord and systemd, so `init: openrc` failed schema validation
// against an init charly fully implements — with its own service_template,
// management_command set and unit_path_template.
//
// This case fails on the pre-change enum: `openrc` is not a member, so unification
// with #Box is a conflict.
func TestBoxInitAcceptsEveryVocabularyInit(t *testing.T) {
	for _, init := range []string{"supervisord", "systemd", "openrc"} {
		if err := boxInit(t, init); err != nil {
			t.Errorf("#Box.init rejects %q, which the build vocabulary defines as a "+
				"first-class init: a box cannot select an init charly implements\n%v", init, err)
		}
	}
}

// The enum must stay an enum. Widening it by loosening the type to `string` would make
// the case above pass while silently accepting typos, so the discrimination is asserted
// directly rather than assumed.
func TestBoxInitStillRejectsAnUnknownInit(t *testing.T) {
	for _, bogus := range []string{"sysvinit", "runit", "Systemd", ""} {
		if err := boxInit(t, bogus); err == nil {
			t.Errorf("#Box.init accepts %q, which no init in the build vocabulary defines — "+
				"the field has stopped constraining anything", bogus)
		}
	}
}

// The doc comment states the invariant the enum has to hold. Keep it: it is the reason
// the next init added to the vocabulary has to be added here too, and it is the only
// place that says so — spec cannot read charly's vocabulary to check it mechanically.
func TestBoxInitDocumentsItsInvariant(t *testing.T) {
	b, err := schema.FS.ReadFile("box.cue")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "the build vocabulary actually defines") {
		t.Error("box.cue no longer states why this enum's membership is constrained; " +
			"without it the next init is omitted the way openrc was")
	}
}
