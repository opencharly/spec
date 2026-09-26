package lock

import (
	"reflect"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestBedVmDomainsAlongsideOnly pins the position-derived filter: the host-global
// libvirt domain contention unit covers ALONGSIDE (deploy-level) vm members only
// — an in-substrate member's domain runs inside its parent's venue.
func TestBedVmDomainsAlongsideOnly(t *testing.T) {
	ssh := &spec.DescentDescriptor{Transport: "ssh", Venue: "ssh", ExclusiveVenue: true}
	node := spec.DeployNode{
		Member: []spec.Member{
			{Name: "peer-vm", Position: spec.PositionDeployLevel, Node: &spec.DeployNode{Descent: ssh}},
			{Name: "inner-vm", Position: spec.PositionInSubstrate, Node: &spec.DeployNode{Descent: ssh}},
		},
	}
	got := BedVmDomains("bed", node)
	want := []string{"charly-" + spec.VmDomainIdentity("peer-vm")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BedVmDomains = %v, want %v (the in-substrate member's domain is not host-global)", got, want)
	}
}

// TestBedVmDomainsExcludesKubeVirt pins the ssh-venue/substrate split (R1): a kubevirt node
// ALSO descends over ssh but is NOT a host libvirt domain, so it must contribute no host-global
// domain lock. The discriminator is the ExclusiveVenue trait, declared for vm and deliberately
// not for kubevirt.
func TestBedVmDomainsExcludesKubeVirt(t *testing.T) {
	kubevirt := &spec.DescentDescriptor{Transport: "ssh", Venue: "ssh", ExclusiveVenue: false}
	node := spec.DeployNode{
		Descent: kubevirt,
		Member: []spec.Member{
			{Name: "kv-vm", Position: spec.PositionDeployLevel, Node: &spec.DeployNode{Descent: kubevirt}},
		},
	}
	if got := BedVmDomains("bed", node); len(got) != 0 {
		t.Fatalf("BedVmDomains = %v, want [] (a kubevirt ssh-venue node holds no host libvirt domain)", got)
	}
}
