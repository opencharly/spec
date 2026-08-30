package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// vmSource compiles the shipped schema and unifies #VmSource with the given source
// literal, returning whether it satisfies the schema.
func vmSource(t *testing.T, src string) error {
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
	def := v.LookupPath(cue.ParsePath("#VmSource"))
	if !def.Exists() {
		t.Fatal("#VmSource is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(src))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(false))
}

// The `iso` arm exists so charly can install a distro that ships ONLY an installer
// medium — no cloud image, no bootstrap tarball. Omarchy is the first: it publishes
// omarchy-<ver>.iso and nothing else.
//
// This literal is the source of a VM that was actually installed end to end: the real
// omarchy-4.0.1.iso booted under QEMU/OVMF with a rendered `cidata` seed, ran its
// installer unattended, rebooted itself, and was reachable over SSH using only the
// seeded key. It is the corpus case, not a hypothetical.
func TestVmSourceAcceptsTheIsoArm(t *testing.T) {
	err := vmSource(t, `{
		kind:   "iso"
		url:    "https://iso.omarchy.org/omarchy-4.0.1.iso"
		distro: "omarchy"
		checksum: {type: "sha256", value: "69cbb4e10d98ad831c3c9f245b5757a9d1fedfd0c9592780e977d6f950dea8c3"}
		install_timeout: "45m"
		installer: {
			username: "user"
			password: "user"
			hostname: "omarchy"
			timezone: "UTC"
			keyboard: "us"
			disk:     "/dev/vda"
			encrypt:  false
			ssh_authorized_key: ["ssh-ed25519 AAAA"]
		}
	}`)
	if err != nil {
		t.Errorf("#VmSource rejects a real, verified iso install source: a distro that "+
			"ships only an installer medium cannot be expressed at all\n%v", err)
	}
}

// The imaging-rig mode: install with no personal details and let whoever boots the
// machine first create their own user. The credential fields are simply absent, so the
// arm must accept an installer block carrying only `defer_provisioning`.
func TestVmSourceAcceptsDeferredProvisioning(t *testing.T) {
	err := vmSource(t, `{
		kind:      "iso"
		url:       "https://iso.omarchy.org/omarchy-4.0.1.iso"
		distro:    "omarchy"
		installer: {defer_provisioning: true}
	}`)
	if err != nil {
		t.Errorf("#VmSource rejects a deferred-provisioning iso source, which is the "+
			"documented prepare-for-another-owner install\n%v", err)
	}
}

// The arm must stay discriminated. Each of these is a field that belongs to a DIFFERENT
// source kind, and accepting one would silently produce a VM built by neither path:
//
//   - rootfs/root_size: the INSTALLER partitions the disk on this arm, not charly, so a
//     filesystem choice here would be read by nothing.
//   - base_user: installer.username is the account; base_user is the cloud_image adopt
//     mechanism and would be silently ignored.
//   - builder/builder_image: the bootstrap arm's builder container.
//   - box: the bootc arm's source image.
func TestVmSourceIsoRejectsCrossBranchFields(t *testing.T) {
	for _, bogus := range []struct{ name, field string }{
		{"rootfs", `rootfs: "btrfs"`},
		{"root_size", `root_size: "40G"`},
		{"base_user", `base_user: "arch"`},
		{"builder", `builder: "pacstrap"`},
		{"builder_image", `builder_image: "omarchy-pacstrap-builder"`},
		{"box", `box: "omarchy"`},
	} {
		err := vmSource(t, `{
			kind:   "iso"
			url:    "https://iso.omarchy.org/omarchy-4.0.1.iso"
			distro: "omarchy"
			`+bogus.field+`
		}`)
		if err == nil {
			t.Errorf("#VmSource.iso accepts %s, which belongs to another source kind and "+
				"would be silently ignored by the iso build path", bogus.name)
		}
	}
}

// distro is CUE-required on this arm, exactly as it is on `bootstrap`. The
// ANSWER-FILE FORMAT lives on the distro (#DistroInstaller), so an absent value is a
// hard nil at render time rather than a wrong default — there is no plausible-default
// failure mode to trade against, which is why this arm follows `bootstrap` rather than
// `cloud_image` (where `distro` is declared optional and presence is enforced by the vm
// kind's own OpValidate instead).
//
// A required-but-absent field is INCOMPLETE, not conflicting, so it surfaces under a
// concreteness check — which is what the host's decode performs. The assertion is
// written as PARITY with the bootstrap arm rather than against a bare mode, so it keeps
// holding if that contract ever moves.
func TestVmSourceIsoRequiresDistroLikeBootstrap(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#VmSource"))

	concrete := func(lit string) error {
		return def.Unify(ctx.CompileString(lit)).Validate(cue.Concrete(true))
	}

	isoNoDistro := concrete(`{kind: "iso", url: "https://iso.omarchy.org/omarchy-4.0.1.iso"}`)
	bootstrapNoDistro := concrete(`{kind: "bootstrap", builder: "pacstrap"}`)

	if bootstrapNoDistro == nil {
		t.Fatal("the bootstrap arm no longer requires distro; this test's parity premise " +
			"is stale and the iso arm's contract needs re-deciding, not just re-asserting")
	}
	if isoNoDistro == nil {
		t.Error("#VmSource.iso accepts a source with no distro while bootstrap rejects it: " +
			"the answer-file format lives on the distro, so this would render nothing and " +
			"fail only at install time")
	}

	// And both accept it once supplied, so the assertion above is about absence, not
	// about the arms being broken.
	if err := concrete(`{kind: "iso", url: "https://iso.omarchy.org/omarchy-4.0.1.iso", distro: "omarchy"}`); err != nil {
		t.Errorf("#VmSource.iso rejects a complete source: %v", err)
	}
}

// A misspelled distro must be a schema conflict, not an extension point — the same rule
// the bootstrap arm states. Without this the id silently falls through to no build
// vocabulary at all.
func TestVmSourceIsoRejectsUnknownDistro(t *testing.T) {
	for _, bogus := range []string{"omarchi", "Omarchy", "notadistro"} {
		err := vmSource(t, `{
			kind:   "iso"
			url:    "https://iso.omarchy.org/omarchy-4.0.1.iso"
			distro: "`+bogus+`"
		}`)
		if err == nil {
			t.Errorf("#VmSource.iso accepts distro %q, which is not in the closed "+
				"#DistroID vocabulary", bogus)
		}
	}
}

// #DistroInstaller is the renderer half. The omarchy entry below is the shape the
// distro entity will carry: the cidata volume label, the two required archinstall answer
// files, and a `when:`-guarded optional one.
func TestDistroInstallerAcceptsTheOmarchyShape(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#DistroInstaller"))
	if !def.Exists() {
		t.Fatal("#DistroInstaller is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(`{
		volume_id: "cidata"
		done:      "poweroff"
		file: [
			{path: "user_configuration.json", content: "{}"},
			{path: "user_credentials.json", content: "{}"},
			{path: "authorized_keys", content: "k", when: "{{if .SSHAuthorizedKeys}}yes{{end}}"},
		]
	}`))
	if unified.Err() != nil {
		t.Fatalf("#DistroInstaller rejects the omarchy answer-file shape: %v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(false)); err != nil {
		t.Errorf("#DistroInstaller rejects the omarchy answer-file shape: %v", err)
	}
}

// The volume label is what the installer greps for; a stray slash or space silently
// produces a volume nothing mounts.
func TestDistroInstallerRejectsAMalformedVolumeID(t *testing.T) {
	schemaSrc, _, _ := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	def := v.LookupPath(cue.ParsePath("#DistroInstaller"))
	for _, bogus := range []string{"ci data", "ci/data", ""} {
		unified := def.Unify(ctx.CompileString(`{
			volume_id: "` + bogus + `"
			file: [{path: "a.json", content: "{}"}]
		}`))
		if unified.Err() == nil && unified.Validate(cue.Concrete(false)) == nil {
			t.Errorf("#DistroInstaller accepts volume_id %q, which no installer will find", bogus)
		}
	}
}

// disk_size_bytes exists because a real installer answer file states partition sizes as
// ABSOLUTE NUMBERS, not as "the rest of the disk". archinstall indexes partition['size']
// with no default and offers no fill-remaining sentinel, so a template that cannot see the
// disk size cannot describe a root partition at all — measured against the shipping
// Omarchy 4.0.1 installer, which aborts with `KeyError: 'size'` without it.
//
// It is an int (bytes), not a suffixed string: the arithmetic happens in the template, and
// a template is the wrong place to parse "40G".
func TestInstallerSeedContextCarriesTheDiskSize(t *testing.T) {
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#InstallerSeedContext"))
	if !def.Exists() {
		t.Fatal("#InstallerSeedContext is not defined in the shipped schema")
	}

	// A 40 GiB disk, as a whole number of bytes.
	unified := def.Unify(ctx.CompileString(`{
		hostname:        "omarchy"
		disk:            "/dev/vda"
		disk_size_bytes: 42949672960
	}`))
	if unified.Err() != nil {
		t.Fatalf("#InstallerSeedContext rejects disk_size_bytes: %v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(false)); err != nil {
		t.Errorf("#InstallerSeedContext rejects disk_size_bytes: %v", err)
	}

	// A suffixed string must NOT satisfy it: the whole point is that the template gets a
	// number it can do arithmetic on, and CUE is where that is enforced rather than in a
	// renderer that would have to fail at execute time, inside a guest nobody is watching.
	bad := def.Unify(ctx.CompileString(`{disk_size_bytes: "40G"}`))
	if bad.Err() == nil && bad.Validate(cue.Concrete(false)) == nil {
		t.Error("#InstallerSeedContext accepts a suffixed string for disk_size_bytes; it must be bytes")
	}
}
