package spec

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestVmBuildRequestFromSnapshotRoundTrip proves the from_snapshot field survives a
// marshal/unmarshal round trip (the R10 gate for the field — the test FAILS without the
// schema field, since the yaml tag would silently drop it).
func TestVmBuildRequestFromSnapshotRoundTrip(t *testing.T) {
	req := VmBuildRequest{Box: "cachyos-vm", FromSnapshot: "golden"}
	data, err := yaml.Marshal(&req)
	if err != nil {
		t.Fatalf("marshalling VmBuildRequest: %v", err)
	}
	var back VmBuildRequest
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshalling VmBuildRequest: %v", err)
	}
	if back.FromSnapshot != "golden" {
		t.Fatalf("from_snapshot did not survive the round trip: got %q want %q (the schema field is missing?)", back.FromSnapshot, "golden")
	}
	if back.Box != "cachyos-vm" {
		t.Fatalf("box did not survive the round trip: got %q", back.Box)
	}
}

// TestCheckBedMemberFromSnapshotRoundTrip proves CheckBedMember.from_snapshot survives too —
// a group bed's VM members carry the unified from: name:tag into their vm-build leg.
func TestCheckBedMemberFromSnapshotRoundTrip(t *testing.T) {
	mem := CheckBedMember{Key: "b", IsVM: true, From: "cachyos-vm", FromSnapshot: "golden"}
	data, err := yaml.Marshal(&mem)
	if err != nil {
		t.Fatalf("marshalling CheckBedMember: %v", err)
	}
	var back CheckBedMember
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshalling CheckBedMember: %v", err)
	}
	if back.FromSnapshot != "golden" {
		t.Fatalf("from_snapshot did not survive the round trip: got %q want golden", back.FromSnapshot)
	}
	if back.From != "cachyos-vm" {
		t.Fatalf("from did not survive the round trip: got %q", back.From)
	}
}

// TestCheckBedReplyFromSnapshotRoundTrip proves CheckBedReply.from_snapshot survives too —
// the runner reads it to thread the snapshot into the vm-build step.
func TestCheckBedReplyFromSnapshotRoundTrip(t *testing.T) {
	rep := CheckBedReply{VMTemplate: "cachyos-vm", FromSnapshot: "golden"}
	data, err := yaml.Marshal(&rep)
	if err != nil {
		t.Fatalf("marshalling CheckBedReply: %v", err)
	}
	var back CheckBedReply
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshalling CheckBedReply: %v", err)
	}
	if back.FromSnapshot != "golden" {
		t.Fatalf("from_snapshot did not survive the round trip: got %q want %q", back.FromSnapshot, "golden")
	}
	if back.VMTemplate != "cachyos-vm" {
		t.Fatalf("vm_template did not survive the round trip: got %q", back.VMTemplate)
	}
}

// TestDeployNodeDelDispatchAncestorsRoundTrip is the R10 gate for the del seam's
// ancestor_paths/ancestor_nodes (opencharly/charly#765): the plugin ships the target's
// ROOT-FIRST ancestor chain as DATA and the host re-derives the parentExec (the VENUE) from
// it. The test FAILS without the schema fields — the yaml tags would silently drop both
// lists, which is exactly the bug: a teardown reaching a FRESH process then has no venue and
// replays an in-substrate member's `package:` reverse ops on the HOST.
func TestDeployNodeDelDispatchAncestorsRoundTrip(t *testing.T) {
	req := DeployNodeDelDispatchRequest{
		Name:          "root.member",
		AssumeYes:     true,
		AncestorPaths: []string{"root"},
		AncestorNodes: []Deploy{{Kind: "vm", MemberOf: "root"}},
	}
	data, err := yaml.Marshal(&req)
	if err != nil {
		t.Fatalf("marshalling DeployNodeDelDispatchRequest: %v", err)
	}
	var back DeployNodeDelDispatchRequest
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshalling DeployNodeDelDispatchRequest: %v", err)
	}
	if len(back.AncestorPaths) != 1 || back.AncestorPaths[0] != "root" {
		t.Fatalf("ancestor_paths did not survive the round trip: got %#v want [root] (the schema field is missing?)", back.AncestorPaths)
	}
	if len(back.AncestorNodes) != 1 || back.AncestorNodes[0].Kind != "vm" || back.AncestorNodes[0].MemberOf != "root" {
		t.Fatalf("ancestor_nodes did not survive the round trip: got %#v (the schema field is missing?)", back.AncestorNodes)
	}
	if back.Name != "root.member" || !back.AssumeYes {
		t.Fatalf("the pre-existing del fields did not survive the round trip: name=%q assume_yes=%v", back.Name, back.AssumeYes)
	}
	// The empty case must stay valid — a TOP-LEVEL node's teardown has no ancestors, so the
	// host must keep its previous (RootExecutorForDeployNode) path.
	empty, err := yaml.Marshal(&DeployNodeDelDispatchRequest{Name: "solo"})
	if err != nil {
		t.Fatalf("marshalling the ancestor-less request: %v", err)
	}
	var backEmpty DeployNodeDelDispatchRequest
	if err := yaml.Unmarshal(empty, &backEmpty); err != nil {
		t.Fatalf("unmarshalling the ancestor-less request: %v", err)
	}
	if len(backEmpty.AncestorPaths) != 0 || len(backEmpty.AncestorNodes) != 0 {
		t.Fatalf("an ancestor-less del request gained ancestors: %#v / %#v", backEmpty.AncestorPaths, backEmpty.AncestorNodes)
	}
}
