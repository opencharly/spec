package deploy

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestVenueSplitIsTraitDriven drives the venue predicates through the REAL derivation
// (spec.DescentFromTraits) with the SAME #DeployTraits the substrate provider
// declares — not a hand-built DescentDescriptor — so it would catch a wrong or
// missing live trait.
//
// The declared trait table is candy/plugin-substrate's substrateTraits (its single
// source, serialized over Describe and stamped by kit.StampDescent). The vm/kubevirt
// rows are mirrored verbatim:
//
//	"vm":       {Venue: "ssh", ...}
//	"kubevirt": {Venue: "kubevirt", ...}
func TestVenueSplitIsTraitDriven(t *testing.T) {
	vm := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "ssh", MachineVenue: true, ExclusiveVenue: true, BedTarget: true})}
	kv := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "kubevirt", ImageBacked: true, BedTarget: true})}

	// Both descend over the ssh transport.
	if vm.Descent.Transport != "ssh" || kv.Descent.Transport != "ssh" {
		t.Fatalf("transport: vm=%q kubevirt=%q — both must be ssh", vm.Descent.Transport, kv.Descent.Transport)
	}
	// Only the host-libvirt vm is the `charly vm` (libvirt-domain) venue.
	if !IsVmVenue(vm) {
		t.Fatal("IsVmVenue(vm) = false; the vm substrate IS the host-libvirt venue")
	}
	if IsVmVenue(kv) {
		t.Fatal("IsVmVenue(kubevirt) = true; kubevirt is a cluster CR, not a host libvirt domain")
	}
	if KubeVirtVenue(vm) {
		t.Fatal("KubeVirtVenue(vm) = true; the vm substrate is host-libvirt")
	}
	if !KubeVirtVenue(kv) {
		t.Fatal("KubeVirtVenue(kubevirt) = false; kubevirt carries its own venue")
	}
	// A non-ssh venue is none of the three.
	pod := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "container"})}
	if IsVmVenue(pod) || KubeVirtVenue(pod) {
		t.Fatal("a container-venue node must be neither vm nor kubevirt")
	}
	if pod.Descent.Transport == "ssh" {
		t.Fatal("a container-venue node must not transport over ssh")
	}
}
