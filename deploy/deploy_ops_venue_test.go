package deploy

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestVenueSplitIsTraitDriven drives the venue behaviour through the REAL derivation
// (spec.DescentFromTraits) with the SAME #DeployTraits the substrate provider declares — not a
// hand-built DescentDescriptor — so it catches a wrong/missing live trait.
//
// It pins TRANSITION SAFETY: IsVmVenue must be false for a kubevirt node in BOTH the
// post-migration trait state (its own `kubevirt` venue) AND the pre-migration state (ssh venue,
// no ExclusiveVenue) — so no consumer's spec/plugin-substrate pin ordering can regress a
// kubevirt node into the libvirt arm. It also pins that the new `kubevirt` venue still descends
// over the ssh TRANSPORT (the venue value is consumed by DescentFromTraits).
func TestVenueSplitIsTraitDriven(t *testing.T) {
	vm := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "ssh", MachineVenue: true, ExclusiveVenue: true, BedTarget: true})}
	kvPost := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "kubevirt", ImageBacked: true, BedTarget: true})}
	kvPre := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "ssh", ImageBacked: true, BedTarget: true})}

	// All three descend over the ssh transport (kubevirt reaches the guest over ssh).
	for name, n := range map[string]*spec.DeployNode{"vm": vm, "kubevirt-post": kvPost, "kubevirt-pre": kvPre} {
		if n.Descent.Transport != "ssh" {
			t.Fatalf("%s: transport = %q, want ssh", name, n.Descent.Transport)
		}
	}
	// The distinct venue is preserved on the stamped descriptor (its value is what a
	// consumer bed arm reads).
	if kvPost.Descent.Venue != "kubevirt" {
		t.Fatalf("kubevirt-post venue = %q, want kubevirt", kvPost.Descent.Venue)
	}
	// Only the host-libvirt vm is the `charly vm` (libvirt-domain) venue — in BOTH kubevirt states.
	if !IsVmVenue(vm) {
		t.Fatal("IsVmVenue(vm) = false; the vm substrate IS the host-libvirt venue")
	}
	if IsVmVenue(kvPost) {
		t.Fatal("IsVmVenue(kubevirt-post) = true; a kubevirt node must never take the libvirt arm")
	}
	if IsVmVenue(kvPre) {
		t.Fatal("IsVmVenue(kubevirt-pre) = true; a pre-migration kubevirt node must never take the libvirt arm")
	}
	// A non-ssh venue is not the vm venue.
	pod := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "container"})}
	if IsVmVenue(pod) {
		t.Fatal("a container-venue node must not be the vm venue")
	}
}
