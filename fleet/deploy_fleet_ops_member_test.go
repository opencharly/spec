package fleet

import (
	"reflect"
	"testing"

	"github.com/opencharly/spec/spec"
)

// memberTreeFixture builds a bed node carrying both member positions: an
// in-substrate local member (zulu), a deploy-level local member (alpha), and an
// in-substrate android member (skipped by the live-ref gather).
func memberTreeFixture() *spec.FleetNode {
	hostRooted := &spec.DescentDescriptor{Transport: "none", HostRooted: true}
	return &spec.FleetNode{
		Descent: &spec.DescentDescriptor{Transport: "ssh"},
		Member: []spec.Member{
			{Name: "zulu", Position: spec.PositionInSubstrate, Node: &spec.FleetNode{Descent: hostRooted}},
			{Name: "alpha", Position: spec.PositionDeployLevel, Node: &spec.FleetNode{Descent: hostRooted}},
			{Name: "android-child", Position: spec.PositionInSubstrate, Node: &spec.FleetNode{Descent: &spec.DescentDescriptor{Transport: "none", Venue: "parent"}}},
		},
	}
}

// TestResolveNodePathUniformMemberTree pins dotted resolution over the ONE
// member list (both positions addressable).
func TestResolveNodePathUniformMemberTree(t *testing.T) {
	roots := map[string]spec.FleetNode{
		"bed": *memberTreeFixture(),
	}
	node, ancestors, err := ResolveNodePath(roots, "bed.zulu")
	if err != nil {
		t.Fatalf("ResolveNodePath(bed.zulu): %v", err)
	}
	if node.Descent == nil {
		t.Fatal("leaf node must carry its stamped descent")
	}
	if len(ancestors) != 1 || ancestors[0].Member == nil {
		t.Fatalf("ancestors = %v, want the bed node", ancestors)
	}
	if _, _, err := ResolveNodePath(roots, "bed.missing"); err == nil {
		t.Fatal("missing member must error")
	}
}

// TestDeployNestedLocalChildrenPosition pins the position-derived filter: only
// IN-SUBSTRATE host-rooted members apply (the deploy-into class); deploy-level
// members are brought up alongside, never here. Authored order is preserved.
func TestDeployNestedLocalChildrenPosition(t *testing.T) {
	var applied []string
	err := DeployNestedLocalChildren("bed", memberTreeFixture(), func(memberKey, dotted string) error {
		applied = append(applied, dotted)
		return nil
	})
	if err != nil {
		t.Fatalf("DeployNestedLocalChildren: %v", err)
	}
	if !reflect.DeepEqual(applied, []string{"bed.zulu"}) {
		t.Fatalf("applied = %v, want [bed.zulu] (in-substrate only, authored order)", applied)
	}
}

// TestBedCheckLiveRefsUniform pins the check-live gather over the member tree:
// in-substrate members in authored order, android (parent venue) skipped.
func TestBedCheckLiveRefsUniform(t *testing.T) {
	refs := BedCheckLiveRefs("bed", memberTreeFixture())
	want := []string{"bed", "bed.zulu"}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
}

// TestMergeFleetNodeMemberTree pins the structural merge: the uniform ordered
// Member tree merges as real tree data (src non-zero wins).
func TestMergeFleetNodeMemberTree(t *testing.T) {
	dst := spec.FleetNode{Target: "pod"}
	src := spec.FleetNode{
		Target: "vm",
		Member: []spec.Member{
			{Name: "zulu", Position: spec.PositionDeployLevel, Node: &spec.FleetNode{}},
		},
	}
	got := MergeFleetNode(dst, src)
	if got.Target != "vm" || len(got.Member) != 1 || got.Member[0].Name != "zulu" {
		t.Fatalf("merge = %+v, want Target vm + [zulu]", got)
	}
}
