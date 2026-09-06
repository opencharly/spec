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
	t.Logf("marshalled: %s", data)
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
