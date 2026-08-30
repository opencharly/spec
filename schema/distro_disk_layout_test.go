package schema_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// diskLayout unifies #DiskLayout with the given literal and reports whether it validates.
func diskLayout(t *testing.T, src string) error {
	t.Helper()
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#DiskLayout"))
	if !def.Exists() {
		t.Fatal("#DiskLayout is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(src))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(true), cue.Final())
}

// CORPUS: the Omarchy layout, which is the reason this def exists. Read off a real
// ISO-installed Omarchy 4.0.1 guest: ESP at /boot (not /boot/efi) and the four subvolumes
// its fstab and limine cmdline reference.
func TestDiskLayout_OmarchyCorpus(t *testing.T) {
	if err := diskLayout(t, `{
		esp_mount_point: "/boot"
		subvolume: [
			{name: "@",     mount_point: "/",                     mount_options: "compress=zstd,noatime"},
			{name: "@home", mount_point: "/home",                 mount_options: "compress=zstd,noatime"},
			{name: "@log",  mount_point: "/var/log",              mount_options: "compress=zstd,noatime"},
			{name: "@pkg",  mount_point: "/var/cache/pacman/pkg", mount_options: "compress=zstd,noatime"},
		]
	}`); err != nil {
		t.Fatalf("the Omarchy disk layout must validate: %v", err)
	}
}

// CORPUS: an entirely empty block is legal — both fields are optional and both default to
// charly's previous behaviour, which is what keeps every existing distro unaffected.
func TestDiskLayout_EmptyIsValid(t *testing.T) {
	if err := diskLayout(t, `{}`); err != nil {
		t.Fatalf("an empty #DiskLayout must validate: %v", err)
	}
}

// CORPUS: esp_mount_point alone, with no subvolumes — a distro that only relocates its ESP.
func TestDiskLayout_EspOnly(t *testing.T) {
	if err := diskLayout(t, `{esp_mount_point: "/boot"}`); err != nil {
		t.Fatalf("esp_mount_point without subvolumes must validate: %v", err)
	}
}

// TEETH: the shapes that must be REJECTED. Each is a way to declare a layout that would
// build a disk nobody can boot, with no error at build time.
func TestDiskLayout_Teeth(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "a relative esp_mount_point would resolve against the builder's cwd",
			src:  `{esp_mount_point: "boot"}`,
		},
		{
			name: "an empty subvolume name cannot be created",
			src:  `{subvolume: [{name: "", mount_point: "/"}]}`,
		},
		{
			name: "a relative subvolume mount_point would mount outside the guest tree",
			src:  `{subvolume: [{name: "@home", mount_point: "home"}]}`,
		},
		{
			name: "a subvolume without a mount_point has nowhere to go",
			src:  `{subvolume: [{name: "@"}]}`,
		},
		{
			name: "a subvolume without a name cannot be created",
			src:  `{subvolume: [{mount_point: "/"}]}`,
		},
		{
			name: "an unknown field is a typo, not an extension point",
			src:  `{esp_path: "/boot"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := diskLayout(t, tc.src); err == nil {
				t.Fatalf("expected %s to be rejected, but it validated", tc.src)
			}
		})
	}
}

// The def has to be reachable from #Distro and #ResolvedDistro, or it is unusable no
// matter how well it validates on its own.
func TestDiskLayout_ReachableFromDistro(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	for _, def := range []string{"#Distro", "#ResolvedDistro"} {
		d := v.LookupPath(cue.ParsePath(def))
		if !d.Exists() {
			t.Fatalf("%s is not defined", def)
		}
		// Optional fields are not returned by LookupPath, so the Fields iterator is
		// used with cue.Optional(true) — the field IS optional and must stay that way.
		it, err := d.Fields(cue.Optional(true), cue.Definitions(true))
		if err != nil {
			t.Fatalf("%s fields: %v", def, err)
		}
		found := false
		for it.Next() {
			if it.Selector().Unquoted() == "disk_layout" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s has no disk_layout field — the def would be unreachable", def)
		}
	}
}

// A distro body carrying the block must validate end to end, not just the def in isolation.
func TestDistro_WithDiskLayout(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	def := v.LookupPath(cue.ParsePath("#Distro"))
	unified := def.Unify(ctx.CompileString(`{
		inherits: "arch"
		inherit_packages: true
		disk_layout: {
			esp_mount_point: "/boot"
			subvolume: [{name: "@", mount_point: "/", mount_options: "compress=zstd"}]
		}
	}`))
	if unified.Err() != nil {
		t.Fatalf("a distro carrying disk_layout must validate: %v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(false)); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

// Guard against the def being silently widened later: mount_options is the only optional
// field on a subvolume, and name/mount_point must stay required.
func TestSubvolume_RequiredFields(t *testing.T) {
	err := diskLayout(t, `{subvolume: [{name: "@", mount_point: "/"}]}`)
	if err != nil {
		t.Fatalf("mount_options must remain optional: %v", err)
	}
	for _, missing := range []string{
		`{subvolume: [{mount_point: "/"}]}`,
		`{subvolume: [{name: "@"}]}`,
	} {
		if err := diskLayout(t, missing); err == nil {
			t.Errorf("expected %s to be rejected", missing)
		} else if !strings.Contains(err.Error(), "incomplete") && !strings.Contains(err.Error(), "not allowed") {
			t.Logf("rejected as expected: %v", err)
		}
	}
}
