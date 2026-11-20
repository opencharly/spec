package schema_test

// os_source_kind_test.go — the coverage for the `os:` and `source:` KINDS.
//
// WHY THIS FILE EXISTS (R7): the PR that adds the two defs can otherwise prove only that they
// COMPILE, and a definition that compiles proves nothing about what it accepts. Every case below
// unifies a CONCRETE value into the shipped def and demands the judgement a value gate would make,
// so each one FAILS if the def is missing, reopened, or loses a required field.
//
// THE CONCRETENESS RULE IS LOAD-BEARING HERE, and `unifiesWith` in capability_port_test.go states
// why: a missing REQUIRED field leaves a value INCOMPLETE rather than bottom, so a plain Validate()
// ACCEPTS it. `os!` and `package_manager!` are exactly that shape — the two fields D-SRC-2 and
// D-CANDY-3 make mandatory — so every reject case below goes through cue.Concrete(true). A test
// built on Validate() alone would pass with the requirements deleted.

import (
	"testing"

	"cuelang.org/go/cue"
)

// TestOSKindClosedness — #Os accepts one complete os and refuses the incompletenesses that
// D-CANDY-3 and D-SRC-2 make unrepresentable.
func TestOSKindClosedness(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#Os"))
	if !def.Exists() {
		t.Fatal("#Os does not resolve in the shipped schema — the `os:` kind is not implemented")
	}

	// --- accept: the shape an authored os node has ---
	for _, tc := range []struct {
		name  string
		value string
	}{
		{
			"a fedora os with its init per role",
			`{
				description: "Fedora 43"
				family: "fedora"
				version: "43"
				package_manager: "dnf"
				init: {system: "systemd", container: "systemd"}
				base_user: "user"
			}`,
		},
		{
			"an alpine os on openrc, no version",
			`{
				description: "Alpine"
				family: "alpine"
				package_manager: "apk"
				init: {system: "openrc", container: "openrc"}
			}`,
		},
		{
			"a bootstrap-capable arch os",
			`{
				description: "Arch"
				family: "arch"
				package_manager: "pacman"
				init: {system: "systemd", container: "systemd"}
				bootstrapper: {pacstrap: "pacstrap"}
				bootstrap: {package: ["base"]}
				disk_layout: {efi: "/boot", root: "/"}
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifiesWith(def, tc.value); err != nil {
				t.Errorf("#Os refused %s: %v", tc.name, err)
			}
		})
	}

	// --- reject: the mandatory fields, and the vocabulary ---
	for _, tc := range []struct {
		name    string
		value   string
		because string
	}{
		{
			"no package_manager",
			`{description: "d", family: "fedora", init: {system: "systemd", container: "systemd"}}`,
			"package_manager! is D-CANDY-3's: the os owns what installs into it",
		},
		{
			"no init",
			`{description: "d", family: "fedora", package_manager: "dnf"}`,
			"init! is D-CANDY-3's: the os owns the init",
		},
		{
			"a package_manager that is not in the vocabulary",
			`{description: "d", family: "fedora", package_manager: "emerge", init: {system: "systemd", container: "systemd"}}`,
			"_packageManager is the closed set charly drives",
		},
		{
			"an init role outside _initRole",
			`{description: "d", family: "fedora", package_manager: "dnf", init: {system: "systemd", guest: "systemd"}}`,
			"#OsInit is closed over _initRole, so a typo in a role key is a schema error, not a silently-ignored service",
		},
		{
			"an unknown field",
			`{description: "d", family: "fedora", package_manager: "dnf", init: {system: "systemd", container: "systemd"}, nosuch: "x"}`,
			"#Os is CLOSED — an unknown key is a typo",
		},
	} {
		t.Run("reject "+tc.name, func(t *testing.T) {
			if err := unifiesWith(def, tc.value); err == nil {
				t.Errorf("#Os ACCEPTED %s — %s", tc.name, tc.because)
			}
		})
	}
}

// TestSourceKindExactlyOneOrigin — D-SRC-1 says a source has exactly ONE origin, and #SourceAlt
// being a key-tagged S4 union is what makes that structural rather than advisory.
func TestSourceKindExactlyOneOrigin(t *testing.T) {
	v := compileShipped(t)
	def := v.LookupPath(cue.ParsePath("#Source"))
	if !def.Exists() {
		t.Fatal("#Source does not resolve in the shipped schema — the `source:` kind is not implemented")
	}

	// --- accept: one origin each, every one naming its os (D-SRC-2) ---
	for _, tc := range []struct {
		name  string
		value string
	}{
		{
			"oci",
			`{oci: "docker.io/library/archlinux:latest", description: "Arch", os: "arch"}`,
		},
		{
			"disk pinned by sha256",
			`{disk: {url: "https://example.invalid/a.qcow2", sha256: "` + repeatHex64() + `"}, description: "d", os: "arch"}`,
		},
		{
			"bootstrap",
			`{bootstrap: {builder: "pacstrap", package: {base: ["base"]}}, description: "d", os: "arch"}`,
		},
		{
			"iso",
			`{iso: {url: "https://example.invalid/a.iso", sha256: "` + repeatHex64() + `"}, description: "d", os: "arch"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := unifiesWith(def, tc.value); err != nil {
				t.Errorf("#Source refused the %s origin: %v", tc.name, err)
			}
		})
	}

	// --- reject: D-SRC-1 and D-SRC-2 ---
	for _, tc := range []struct {
		name    string
		value   string
		because string
	}{
		{
			"two origins",
			`{oci: "docker.io/library/archlinux:latest", bootstrap: {builder: "pacstrap"}, description: "d", os: "arch"}`,
			"D-SRC-1: exactly one origin — the S4 union is what enforces it",
		},
		{
			"zero origins",
			`{description: "d", os: "arch"}`,
			"D-SRC-1: exactly one origin",
		},
		{
			"no os",
			`{oci: "docker.io/library/archlinux:latest", description: "d"}`,
			"D-SRC-2: every source names the os that owns its conversions",
		},
		{
			"an unpinned remote disk",
			`{disk: {url: "https://example.invalid/a.qcow2"}, description: "d", os: "arch"}`,
			"D-SRC-1: a remote origin is pinned by sha256, so an unpinned one is unrepresentable",
		},
	} {
		t.Run("reject "+tc.name, func(t *testing.T) {
			if err := unifiesWith(def, tc.value); err == nil {
				t.Errorf("#Source ACCEPTED %s — %s", tc.name, tc.because)
			}
		})
	}
}

// repeatHex64 is a sha256-shaped literal, so a #Sha256 field is satisfied by something the regex
// admits rather than by a value chosen to make the case pass for the wrong reason.
func repeatHex64() string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
