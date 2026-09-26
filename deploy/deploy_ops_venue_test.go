package deploy

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// These tests drive the predicates through the REAL derivation
// (spec.DescentFromTraits) with the SAME #DeployTraits the substrate provider
// declares — not a hand-built DescentDescriptor — so they would catch a wrong or
// missing live trait (the R1/B14(d) concern the earlier hand-built fixture could not).
//
// The declared trait table is candy/plugin-substrate's substrateTraits (its single
// source, serialized over Describe and stamped by kit.StampDescent). The vm/kubevirt
// rows are mirrored verbatim here so the predicate is exercised against the exact
// trait VALUES the provider ships:
//
//	"vm":       {Venue: "ssh", MachineVenue: true, ExclusiveVenue: true, ...}
//	"kubevirt": {Venue: "ssh", ImageBacked: true, BedTarget: true, ...}   // NO ExclusiveVenue
func TestSshVenueSplitIsTraitDriven(t *testing.T) {
	vmTraits := &spec.DeployTraits{Venue: "ssh", MachineVenue: true, ExclusiveVenue: true, BedTarget: true}
	kvTraits := &spec.DeployTraits{Venue: "ssh", ImageBacked: true, BedTarget: true}

	vm := &spec.DeployNode{Descent: spec.DescentFromTraits(vmTraits)}
	kv := &spec.DeployNode{Descent: spec.DescentFromTraits(kvTraits)}

	// Both ride the ssh transport hop.
	if !SshVenue(vm) || !SshVenue(kv) {
		t.Fatalf("SshVenue: vm=%v kubevirt=%v — both must be the ssh venue", SshVenue(vm), SshVenue(kv))
	}
	// Only the host-libvirt vm is a machine-venue / exclusive host lease.
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
		t.Fatal("KubeVirtVenue(kubevirt) = false; kubevirt is the plugin-owned ssh venue")
	}
	// A non-ssh venue is neither.
	pod := &spec.DeployNode{Descent: spec.DescentFromTraits(&spec.DeployTraits{Venue: "container"})}
	if IsVmVenue(pod) || KubeVirtVenue(pod) || SshVenue(pod) {
		t.Fatal("a container-venue node must be neither vm nor kubevirt")
	}
}
