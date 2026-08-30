package spec

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// ResolveInherits is a hand-written struct literal — twice, once per branch — and a
// hand-written copy of a growing struct is a silent-drop hazard. It has already dropped
// four fields: DiskLayout, Installer, InheritPackages and Raw.
//
// The failure has no error path. A distro that declares `inherits:` simply arrives at its
// consumer with those fields nil, and the consumer takes its default branch. Every gate in
// the chain stays green:
//
//   - the btrfs subvolume feature (this repo's #DiskLayout/#Subvolume, plugin-vm's
//     EmitDiskBuildScript arm, charly's omarchy entity) did nothing at all for any distro
//     with a parent — which is every derivative, the case the feature was built for.
//   - source.kind: iso could not resolve an installer for a derived distro, reporting
//     "declares no installer" against a distro that plainly declares one.
//   - inherit_packages — the switch the whole distro TAG CHAIN rests on — was dropped from
//     the resolved value, so a consumer reading it post-resolution saw false.
//   - Raw, the authored body consumers fall back to for anything this projection does not
//     model, became empty for any distro with a parent, turning "not modelled" into "not
//     present".
//
// This test fails on ANY field a future spec addition forgets, which is the only durable
// answer to a hand-written copy that must track a struct.
func TestResolveInherits_CarriesEveryField(t *testing.T) {
	// Parent declares NOTHING the child also declares, so anything present on the result
	// came from the child's own copy rather than from inheritance. That separation is what
	// makes a zero value here unambiguously a drop.
	parent := &ResolvedDistro{Version: "parent-version"}

	child := &ResolvedDistro{
		Inherits:        "parent",
		InheritPackages: true,
		Version:         "child-version",
		Bootstrap:       Bootstrap{InstallCmd: "pacman -S"},
		Workarounds:     []string{"no-check-certificate"},
		Format:          map[string]*Format{"pac": {}},
		BaseUser:        &BaseUser{Name: "user", UID: 1000, GID: 1000, Home: "/home/user"},
		Pacstrap:        &Pacstrap{BasePackages: []string{"base"}},
		Debootstrap:     &Debootstrap{Suite: "trixie"},
		AlpineBootstrap: &AlpineBootstrap{MirrorURL: "https://dl-cdn.alpinelinux.org/alpine"},
		Bootloader:      &Bootloader{InstallTemplate: "echo install"},
		DiskLayout:      &DiskLayout{EspMountPoint: "/boot"},
		Installer:       &DistroInstaller{VolumeID: "cidata"},
		Dnf:             &Dnf{MaxParallelDownloads: 8, Fastestmirror: true},
		Raw:             json.RawMessage(`{"authored":"body"}`),
	}

	dc := &DistroConfig{Distro: map[string]*ResolvedDistro{"parent": parent}}
	got := dc.ResolveInherits(child, 10)

	rv := reflect.ValueOf(*got)
	rt := rv.Type()
	var dropped []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		if rv.Field(i).IsZero() {
			dropped = append(dropped, f.Name)
		}
	}
	sort.Strings(dropped)
	if len(dropped) > 0 {
		t.Fatalf("ResolveInherits dropped %d field(s) the child declared: %v\n"+
			"Every one was set on the child above and arrived zero. Add it to BOTH merged "+
			"struct literals in ResolveInherits (there are two branches — child-with-bootstrap "+
			"and child-without). If a field genuinely must not be carried, say why HERE — do "+
			"not delete this test.", len(dropped), dropped)
	}
}

// The same guard for the OTHER branch. ResolveInherits has two merged literals — one when
// the child declares its own bootstrap, one when it inherits the parent's — and a fix
// applied to only one of them would pass the test above while leaving half the callers
// broken. Which branch runs depends solely on whether def.Bootstrap.InstallCmd is set.
func TestResolveInherits_CarriesEveryField_InheritedBootstrapBranch(t *testing.T) {
	parent := &ResolvedDistro{
		Version:   "parent-version",
		Bootstrap: Bootstrap{InstallCmd: "pacman -S"},
		// Workarounds comes from the PARENT on this branch by design.
		Workarounds: []string{"no-check-certificate"},
	}
	child := &ResolvedDistro{
		Inherits:        "parent",
		InheritPackages: true,
		Version:         "child-version",
		// No Bootstrap — this is what selects the second branch.
		Format:          map[string]*Format{"pac": {}},
		BaseUser:        &BaseUser{Name: "user", UID: 1000, GID: 1000, Home: "/home/user"},
		Pacstrap:        &Pacstrap{BasePackages: []string{"base"}},
		Debootstrap:     &Debootstrap{Suite: "trixie"},
		AlpineBootstrap: &AlpineBootstrap{MirrorURL: "https://dl-cdn.alpinelinux.org/alpine"},
		Bootloader:      &Bootloader{InstallTemplate: "echo install"},
		DiskLayout:      &DiskLayout{EspMountPoint: "/boot"},
		Installer:       &DistroInstaller{VolumeID: "cidata"},
		Dnf:             &Dnf{MaxParallelDownloads: 8, Fastestmirror: true},
		Raw:             json.RawMessage(`{"authored":"body"}`),
	}

	dc := &DistroConfig{Distro: map[string]*ResolvedDistro{"parent": parent}}
	got := dc.ResolveInherits(child, 10)

	if got.Bootstrap.InstallCmd != "pacman -S" {
		t.Fatalf("this test must exercise the INHERITED-bootstrap branch; got bootstrap %+v", got.Bootstrap)
	}

	rv := reflect.ValueOf(*got)
	rt := rv.Type()
	var dropped []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		if rv.Field(i).IsZero() {
			dropped = append(dropped, f.Name)
		}
	}
	sort.Strings(dropped)
	if len(dropped) > 0 {
		t.Fatalf("ResolveInherits (inherited-bootstrap branch) dropped %d field(s): %v\n"+
			"Note this is the SECOND merged literal — a fix applied only to the first one "+
			"passes the other parity test and still breaks every child that omits bootstrap.",
			len(dropped), dropped)
	}
}

// disk_layout and installer must INHERIT, not merely survive. A derivative that adds an
// installer without restating its parent's disk layout is the ordinary case — an Arch spin
// reusing archinstall — and it must not have to copy the parent's whole boot chain.
func TestResolveInherits_DiskLayoutAndInstallerInheritFromParent(t *testing.T) {
	parent := &ResolvedDistro{
		Version:    "parent",
		DiskLayout: &DiskLayout{EspMountPoint: "/boot", Subvolume: []Subvolume{{Name: "@", MountPoint: "/"}}},
		Installer:  &DistroInstaller{VolumeID: "cidata"},
	}
	child := &ResolvedDistro{Inherits: "parent", Bootstrap: Bootstrap{InstallCmd: "pacman -S"}}

	dc := &DistroConfig{Distro: map[string]*ResolvedDistro{"parent": parent}}
	got := dc.ResolveInherits(child, 10)

	if got.DiskLayout == nil {
		t.Fatal("a child with no disk_layout must inherit its parent's")
	}
	if got.DiskLayout.EspMountPoint != "/boot" || len(got.DiskLayout.Subvolume) != 1 {
		t.Errorf("inherited disk_layout is not intact: %+v", got.DiskLayout)
	}
	if got.Installer == nil {
		t.Fatal("a child with no installer must inherit its parent's")
	}
	if got.Installer.VolumeID != "cidata" {
		t.Errorf("inherited installer is not intact: %+v", got.Installer)
	}
}

// A child's OWN disk_layout and installer win over the parent's. Inheritance must not
// overwrite a deliberate override — the direction that would be worse, because it
// silently ignores what the author wrote.
func TestResolveInherits_ChildOverridesParent(t *testing.T) {
	parent := &ResolvedDistro{
		Version:    "parent",
		DiskLayout: &DiskLayout{EspMountPoint: "/boot/efi"},
		Installer:  &DistroInstaller{VolumeID: "OEMDRV"},
	}
	child := &ResolvedDistro{
		Inherits:   "parent",
		Bootstrap:  Bootstrap{InstallCmd: "pacman -S"},
		DiskLayout: &DiskLayout{EspMountPoint: "/boot"},
		Installer:  &DistroInstaller{VolumeID: "cidata"},
	}

	dc := &DistroConfig{Distro: map[string]*ResolvedDistro{"parent": parent}}
	got := dc.ResolveInherits(child, 10)

	if got.DiskLayout.EspMountPoint != "/boot" {
		t.Errorf("the child's disk_layout must win\n got: %q\nwant: /boot", got.DiskLayout.EspMountPoint)
	}
	if got.Installer.VolumeID != "cidata" {
		t.Errorf("the child's installer must win\n got: %q\nwant: cidata", got.Installer.VolumeID)
	}
}
