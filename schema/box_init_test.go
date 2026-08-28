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

// unifyDef validates a concrete value against a named definition in the shipped schema.
func unifyDef(t *testing.T, def, value string) error {
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
	u := d.Unify(ctx.CompileString(value))
	if u.Err() != nil {
		return u.Err()
	}
	return u.Validate(cue.Concrete(false))
}

// The portable lifecycle fields must be authorable on a service. Each is honoured by
// at least two of the three inits — the admission rule that keeps a single-init knob
// out of the shared schema and in unit_options: instead.
func TestCandyServiceAcceptsPortableLifecycleFields(t *testing.T) {
	for _, tc := range []struct{ name, val string }{
		{"type", `{name: "x", exec: "/bin/x", type: "notify"}`},
		{"requires", `{name: "x", exec: "/bin/x", requires: ["a.service"]}`},
		{"restart_sec as a string", `{name: "x", exec: "/bin/x", restart_sec: "5s"}`},
		{"restart_sec as a bare int", `{name: "x", exec: "/bin/x", restart_sec: 5}`},
		{"watchdog_sec", `{name: "x", exec: "/bin/x", watchdog_sec: "30s"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifyDef(t, "#CandyService", tc.val); err != nil {
				t.Errorf("#CandyService rejects %s:\n%v", tc.name, err)
			}
		})
	}
}

// type: is an enum, not free text — a typo must not reach a template and render an
// invalid Type= directive.
func TestCandyServiceTypeIsConstrained(t *testing.T) {
	if err := unifyDef(t, "#CandyService", `{name: "x", exec: "/bin/x", type: "notifyy"}`); err == nil {
		t.Error("#CandyService accepts an unknown service type; the field constrains nothing")
	}
}

// unit_options is the one escape hatch for init-specific directives. It must take a
// scalar OR a list per directive — systemd repeats directives such as
// RuntimeDirectory= once per element — and it must be keyed by init name, so a
// template reads only its own.
func TestCandyServiceUnitOptionsTakeScalarsAndLists(t *testing.T) {
	val := `{
		name: "x", exec: "/bin/x",
		unit_options: {
			systemd: {
				KillMode: "process"
				RuntimeDirectory: ["cstream", "cstream/leaders"]
			}
			openrc: {supervise_daemon_args: "--foo"}
		}
	}`
	if err := unifyDef(t, "#CandyService", val); err != nil {
		t.Errorf("#CandyService rejects unit_options with a scalar and a list:\n%v", err)
	}
}

// The render context is what every init's template renders against, so the same
// fields have to reach it — a field on the entry that never lands in the context is
// unreachable from any template.
func TestServiceRenderContextCarriesTheSameFields(t *testing.T) {
	val := `{
		name: "x", exec: "/bin/x", type: "notify", requires: ["a.service"],
		restart_sec: "5s", watchdog_sec: "30s",
		unit_options: {systemd: {Slice: "session.slice"}}
	}`
	if err := unifyDef(t, "#ServiceRenderContext", val); err != nil {
		t.Errorf("#ServiceRenderContext cannot carry the portable fields:\n%v", err)
	}
}
