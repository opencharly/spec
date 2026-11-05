package spec

import (
	"encoding/json"
	"testing"
)

// TestSaveDeployStateInput_CarriesEngine pins the wire field opencharly/charly#1109 needs:
// a pod deploy's authored `engine:` selector must survive the PrepareVenue state patch that
// plugin-fleet decodes and sdk's SaveDeployState applies. Before this field existed the only
// thing the patch could carry was resolved_image, so the selector was silently dropped and
// `charly config` fell back to the host-wide engine.run (measured: a podman quadlet for an
// `engine: containerd` deploy).
//
// This test FAILS to compile without the field, and fails at run time if the JSON key is
// renamed away from the authored `engine` spelling.
func TestSaveDeployStateInput_CarriesEngine(t *testing.T) {
	in := SaveDeployStateInput{ResolvedImage: "overlay:abc", Engine: EngineName("containerd")}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal SaveDeployStateInput: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal the marshalled state patch: %v", err)
	}
	got, ok := wire["engine"]
	if !ok {
		t.Fatalf("the state patch has no `engine` key: %s", body)
	}
	if got != "containerd" {
		t.Fatalf("engine = %v, want containerd (patch: %s)", got, body)
	}

	var back SaveDeployStateInput
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("decode the state patch: %v", err)
	}
	if back.Engine != EngineName("containerd") {
		t.Fatalf("decoded Engine = %q, want containerd", back.Engine)
	}
}

// TestSaveDeployStateInput_OmitsUnsetEngine is the negative control for the test above: an
// empty selector must stay absent, so sdk's applyDeployState can distinguish "no selection"
// from "select podman" and never clear an operator's existing entry.
func TestSaveDeployStateInput_OmitsUnsetEngine(t *testing.T) {
	body, err := json.Marshal(SaveDeployStateInput{ResolvedImage: "overlay:abc"})
	if err != nil {
		t.Fatalf("marshal SaveDeployStateInput: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal the marshalled state patch: %v", err)
	}
	if _, ok := wire["engine"]; ok {
		t.Fatalf("an unset engine was emitted as present: %s", body)
	}
}
