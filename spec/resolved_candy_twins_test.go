package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestResolvedCandyTwinsAreTheDecodableWireShapes pins the authored/wire split for the candy
// sub-shapes, the shape DESIGN.md 3.1 + 10.2 require together:
//
//   - the AUTHORED defs (#CandyAlias, #CandyVolume) are the shapes a user writes, and 3.1 assigns
//     them to the plugin that owns the candy body;
//   - the RESOLVED twins are what a consumer decodes an opaque body INTO, and 10.2 assigns them to
//     spec.
//
// The property that makes the split work is WIRE COMPATIBILITY: a body authored against the
// authored def must decode into its twin, because that is exactly what happens at runtime — the
// plugin validates the authored shape, the host encodes it, and a consumer decodes the twin. If the
// two ever stopped agreeing on field names, the body would decode to zero values and every test
// that builds fixtures by hand would still pass.
func TestResolvedCandyTwinsAreTheDecodableWireShapes(t *testing.T) {
	cases := []struct {
		name     string
		authored any
		twin     func([]byte) error
		check    func(*testing.T, any)
	}{
		{
			name:     "alias",
			authored: CandyAlias{Name: "rg", Command: "ripgrep"},
			twin: func(b []byte) error {
				var v ResolvedCandyAlias
				return json.Unmarshal(b, &v)
			},
			check: func(t *testing.T, got any) {
				v := got.(ResolvedCandyAlias)
				if v.Name != "rg" || v.Command != "ripgrep" {
					t.Fatalf("decoded %+v, want the authored values", v)
				}
			},
		},
		{
			name:     "volume",
			authored: CandyVolume{Name: "workspace", Path: "/workspace"},
			twin: func(b []byte) error {
				var v ResolvedCandyVolume
				return json.Unmarshal(b, &v)
			},
			check: func(t *testing.T, got any) {
				v := got.(ResolvedCandyVolume)
				if v.Name != "workspace" || v.Path != "/workspace" {
					t.Fatalf("decoded %+v, want the authored values", v)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+" decodes from the authored body", func(t *testing.T) {
			body, err := json.Marshal(tc.authored)
			if err != nil {
				t.Fatalf("marshal the authored value: %v", err)
			}
			if err := tc.twin(body); err != nil {
				t.Fatalf("a body authored against the authored def does not decode into the twin: %v", err)
			}
			var got any
			switch tc.name {
			case "alias":
				var v ResolvedCandyAlias
				_ = json.Unmarshal(body, &v)
				got = v
			case "volume":
				var v ResolvedCandyVolume
				_ = json.Unmarshal(body, &v)
				got = v
			}
			tc.check(t, got)
		})
	}

	t.Run("the twins are not aliases of the authored defs", func(t *testing.T) {
		// They must be DISTINCT types: an alias would make the twin the authored def again, and
		// moving the authored def would take the decode target with it.
		if reflect.TypeOf(ResolvedCandyAlias{}) == reflect.TypeOf(CandyAlias{}) {
			t.Fatal("ResolvedCandyAlias is the same type as CandyAlias — the split did not happen")
		}
		if reflect.TypeOf(ResolvedCandyVolume{}) == reflect.TypeOf(CandyVolume{}) {
			t.Fatal("ResolvedCandyVolume is the same type as CandyVolume — the split did not happen")
		}
	})
}
