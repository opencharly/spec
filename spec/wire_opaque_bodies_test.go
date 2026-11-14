package spec

import (
	"encoding/json"
	"testing"
)

// TestStandaloneLoadCarriesOpaqueBytes pins the load wire's shape: the ONE carrier is
// bytes, and the authored bodies that used to ride it are gone. It fails to COMPILE
// against the pre-cutover generated type (which had Deploy/Box/Candy/Template fields),
// and it fails at run time if a future change reintroduces a typed arm.
func TestStandaloneLoadCarriesOpaqueBytes(t *testing.T) {
	for _, shape := range []string{"deploy", "template", "candy-image", "candy-layer"} {
		in := StandaloneLoad{Shape: shape, Body: RawBody(`{"from":"x"}`)}
		raw, err := json.Marshal(&in)
		if err != nil {
			t.Fatalf("marshal %s: %v", shape, err)
		}
		var out StandaloneLoad
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal %s: %v", shape, err)
		}
		if out.Shape != shape {
			t.Errorf("shape round-trip: got %q want %q", out.Shape, shape)
		}
		if string(out.Body) != `{"from":"x"}` {
			t.Errorf("body round-trip for %s: got %q", shape, string(out.Body))
		}
	}
}

// TestStructuralKindLoadEnvMembersAreBytes pins the members channel: every pre-decoded
// member rides bytes, so the contract cannot name an authored deploy node on the wire.
func TestStructuralKindLoadEnvMembersAreBytes(t *testing.T) {
	in := StructuralKindLoadEnv{
		Members: map[string]RawBody{"pg": RawBody(`{"from":"postgres"}`)},
	}
	raw, err := json.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out StructuralKindLoadEnv
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(out.Members["pg"]) != `{"from":"postgres"}` {
		t.Errorf("member bytes lost: %q", string(out.Members["pg"]))
	}
}
