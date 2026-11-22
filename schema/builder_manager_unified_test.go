package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// builder_manager_unified_test.go — the schema half of the unified builder mechanism
// (opencharly/sdk#608). A build-time manager is a BUILDER, never a box build format or a
// distro format, and every builder word the build vocabulary defines is a member of
// #BuildType. Each case below fails on the pre-change schema.

func unify(t *testing.T, def, doc string) error {
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
	u := d.Unify(ctx.CompileString(doc))
	if u.Err() != nil {
		return u.Err()
	}
	return u.Validate(cue.Concrete(false))
}

// `aur` is a manager, so a box's build: list no longer accepts it: a declared aur package needs a
// builder on every image, not an opt-in format that silently skipped it when absent.
func TestBoxBuildRejectsTheAurManagerAsAFormat(t *testing.T) {
	if err := unify(t, "#Box", `{build: ["pac", "aur"]}`); err == nil {
		t.Fatal("#Box.build accepts `aur`: a build-time manager would still be an opt-in box format that silently drops its packages when absent")
	}
	if err := unify(t, "#Box", `{build: ["pac"]}`); err != nil {
		t.Fatalf("#Box.build rejects a plain OS format: %v", err)
	}
}

// A distro format is an OS package format; `secondary` existed only to let `aur` pose as one.
func TestDistroFormatHasNoSecondaryFlag(t *testing.T) {
	if err := unify(t, "#Format", `{secondary: true}`); err == nil {
		t.Fatal("#Format still accepts `secondary`: a build-time manager could again masquerade as a distro format")
	}
}

// Every builder the vocabulary defines is a #BuildType, mise included (it was missing).
func TestBuildTypeCoversEveryBuilder(t *testing.T) {
	for _, w := range []string{"pixi", "npm", "cargo", "aur", "mise"} {
		if err := unify(t, "#BuildType", `"`+w+`"`); err != nil {
			t.Errorf("#BuildType rejects builder %q: %v", w, err)
		}
	}
}

// An aur declaration has no `replace:`: it was loaded but never applied by either mode, and no
// repo authors it — a field the install silently ignores is deleted, not kept (R5, sdk#608).
func TestAurDeclarationHasNoReplace(t *testing.T) {
	if err := unify(t, "#AUR", `{package: ["kdotool"], replace: ["x"]}`); err == nil {
		t.Fatal("#AUR still accepts replace: a declaration the install ignores")
	}
}

// The package output is the builder PLUGIN's own declaration, returned in its replies — never an
// authored field of the builder node, whose DESIGN shape is a manager map (#BuilderAlt under D-PLUG-2). The
// format is a real OS package format; detect_config is gone (a manager declaration is keyed by
// builder word on its tag section).
func TestBuilderPackageOutputIsDeclaredByThePlugin(t *testing.T) {
	out := `{package: {format: "pac", dir: "/tmp/aur-pkgs/kde-shell", glob: "*.pkg.tar.zst"}}`
	for _, def := range []string{"#BuilderResolveReply", "#BuilderCollectReply"} {
		if err := unify(t, def, out); err != nil {
			t.Errorf("%s rejects the plugin's package output: %v", def, err)
		}
	}
	if err := unify(t, "#BuilderResolveReply", `{package: {format: "zip", dir: "/x", glob: "*"}}`); err == nil {
		t.Error("#BuilderResolveReply accepts an unknown package format")
	}
	if err := unify(t, "#Builder", out); err == nil {
		t.Error("#Builder accepts an authored package output: install behaviour belongs to the builder plugin, not the authored node")
	}
	if err := unify(t, "#Builder", `{detect_config: "aur"}`); err == nil {
		t.Fatal("#Builder still accepts detect_config: a builder could again be triggered by a config section outside the tag-scoped managers")
	}
	// What a builder copies into the target is the plugin's reply, never authored: an authored copy
	// would load and then be ignored, because nothing reads it (opencharly/sdk#608).
	for _, authored := range []string{
		`{copy_artifact: [{src: "/home/u", dst: "/home/u"}]}`,
		`{copy_binary: {src: "/usr/local/bin/pixi", dst: "/usr/local/bin/pixi"}}`,
	} {
		if err := unify(t, "#Builder", authored); err == nil {
			t.Errorf("#Builder accepts %s: an authored copy is loaded and then ignored; the payload is the builder plugin's reply", authored)
		}
	}
}

// A manager declaration lives on the distro tag it was authored under, keyed by manager word.
func TestTagSectionCarriesItsManagerDeclarations(t *testing.T) {
	if err := unify(t, "#TagPkgConfig", `{managers: {aur: {packages: ["kdotool"]}}}`); err != nil {
		t.Fatalf("#TagPkgConfig rejects a manager declaration: %v", err)
	}
	if err := unify(t, "#TagPkgConfig", `{managers: {pip3: {packages: ["x"]}}}`); err == nil {
		t.Fatal("#TagPkgConfig.managers accepts a word that is no builder")
	}
	if err := unify(t, "#CandyModel", `{format_sections: {aur: {packages: ["kdotool"]}}}`); err == nil {
		t.Fatal("#CandyModel still accepts format_sections: a manager section could again be merged image-independently")
	}
}

// The collect input carries the candy's detected manifest, so a builder can declare its teardown
// from it (D-IR-2) instead of an empty reverse.
func TestBuilderCollectInputCarriesTheManifest(t *testing.T) {
	if err := unify(t, "#BuilderCollectInput", `{candy: "c", builder: "npm", manifest: "package.json", manifest_content: "{}"}`); err != nil {
		t.Fatalf("#BuilderCollectInput rejects the manifest: %v", err)
	}
}

// The walk envelope carries the embedded-defaults digest, so the materialized-tree cache (keyed on
// the envelope) re-keys when the binary's default vocabulary changes.
func TestLoadedProjectCarriesTheEmbeddedDefaultsDigest(t *testing.T) {
	if err := unify(t, "#LoadedProject", `{embedded_defaults_digest: "sha256:abc"}`); err != nil {
		t.Fatalf("#LoadedProject rejects the embedded-defaults digest: %v", err)
	}
}
