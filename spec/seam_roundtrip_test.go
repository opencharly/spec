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
