package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestVmBoxLabelCompleteness — the VM analog of the pod-side
// TestCapabilityLabelCompleteness (sdk/deploykit/capabilities_test.go): verifies every
// exported field on spec.VmBoxMetadata has a VmBoxLabelMap entry. Adding a new VM-box
// metadata field without a label mapping is a build break — enforces the invariant
// "every VM-box metadata field rides the ai.opencharly.vm.box label" so a future
// `charly fleet from-box vm:<ref>` can reconstruct the full contract from a pushed box
// image.
func TestVmBoxLabelCompleteness(t *testing.T) {
	if err := CheckVmBoxLabelCompleteness(); err != nil {
		t.Fatal(err)
	}
}

// TestVmBoxMetadataLabelRoundTrip proves the whole VmBoxMetadata struct round-trips
// through its single JSON OCI label (ai.opencharly.vm.box): marshal → unmarshal is an
// identity. The box emitter writes exactly this JSON as the label value (EmitVmBox,
// sdk/deploykit) and the source-less VM deploy reads the struct back from it
// (VmCapabilitiesFromLabels) — this test pins that the Go wire form survives the trip.
func TestVmBoxMetadataLabelRoundTrip(t *testing.T) {
	in := VmBoxMetadata{
		Distro:        "fedora",
		Arch:          "x86_64",
		BaseUser:      "fedora",
		SSHUser:       "charly",
		Firmware:      "uefi-secure",
		Init:          "systemd",
		CharlyInstall: "scp",
		Version:       "0.2026245.0",
		Source: VmBoxSource{
			Kind:         "clone",
			FromVm:       "base-vm",
			FromSnapshot: "snap-1",
		},
		Description: "fedora 43 layered base VM",
		Plan: []Step{
			{Run: "true"},
		},
	}

	wire, err := json.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal VmBoxMetadata: %v", err)
	}

	var out VmBoxMetadata
	if err := json.Unmarshal(wire, &out); err != nil {
		t.Fatalf("unmarshal VmBoxMetadata: %v\nwire: %s", err, wire)
	}

	if !reflect.DeepEqual(in, out) {
		t.Errorf("VmBoxMetadata did not round-trip through JSON:\n in: %+v\nout: %+v", in, out)
	}
}
