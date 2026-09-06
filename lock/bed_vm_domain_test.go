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
	ssh := &spec.DescentDescriptor{Transport: "ssh", Venue: "ssh"}
	node := spec.FleetNode{
		Member: []spec.Member{
			{Name: "peer-vm", Position: spec.PositionDeployLevel, Node: &spec.FleetNode{Descent: ssh}},
			{Name: "inner-vm", Position: spec.PositionInSubstrate, Node: &spec.FleetNode{Descent: ssh}},
		},
	}
	got := BedVmDomains("bed", node)
	want := []string{"charly-" + spec.VmDomainIdentity("peer-vm")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BedVmDomains = %v, want %v (the in-substrate member's domain is not host-global)", got, want)
	}
}
