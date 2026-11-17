package proto_test

// bind_wire_test.go — the R7 proof for the `bind` wire seam (P09 unit 1).
//
// The contract row this makes callable: DESIGN.md:413 — "| binding | core → plugin | a need or use
// paired with its provide, delivered by `bind` to the owner of the capability type (§8) |". The
// wire method was zero in plugin.proto because nobody had wired it, not because the design decided
// against it; this test is the boundary the plugin will actually cross.
//
// It is a REAL round trip through the generated stubs: the CUE shapes from schema/bind.cue are
// marshalled to JSON, carried in the message's bytes field, marshalled as a protobuf, unmarshalled,
// decoded back, and the payload is then VALIDATED against the shipped CUE — because a payload that
// is merely transported is a byte pipe, and one that is validated at the boundary is a contract.

import (
	"bytes"
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"google.golang.org/protobuf/proto"

	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/spec"
)

// goodBindInput is a well-formed #BindInput: a GPU need, the provide that won, and the deployment
// the need was authored in.
func goodBindInput() spec.BindInput {
	return spec.BindInput{
		Need:        spec.BindNeed{Type: "gpu", Name: "nvidia", Config: spec.Opaque{"vendor": "nvidia"}},
		Provided_by: "podman",
		Deployment:  "bed",
	}
}

// shippedSchema compiles the shipped schema/*.cue, the same contract the runtime validates with.
func shippedSchema(t *testing.T) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	v := cuecontext.New().CompileString(src)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	return v
}

// validateAgainst validates a JSON payload against a shipped CUE definition — the check a type
// owner performs when the payload arrives.
func validateAgainst(t *testing.T, def string, raw []byte) error {
	t.Helper()
	v := shippedSchema(t).LookupPath(cue.ParsePath(def))
	if !v.Exists() {
		t.Fatalf("%s does not resolve in the shipped schema", def)
	}
	probe := v.Context().CompileString(string(raw))
	if probe.Err() != nil {
		return probe.Err()
	}
	return v.Unify(probe).Validate(cue.Concrete(true))
}

// carry puts a JSON payload into a message's bytes field, marshals the message, unmarshals it, and
// returns the bytes field the far side would read — the whole wire path, no server needed.
func carry(t *testing.T, raw []byte, build func([]byte) proto.Message, field func(proto.Message) []byte) []byte {
	t.Helper()
	wire, err := proto.Marshal(build(raw))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back := build(nil)
	if err := proto.Unmarshal(wire, back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := field(back); !bytes.Equal(got, raw) {
		t.Fatalf("the bytes did not survive transport: %q", got)
	}
	return field(back)
}

// decodeAs is the far side's decode: the JSON payload back into the CUE-generated shape.
func decodeAs[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// TestBindRoundTripsTheMergedShapes is the positive half: every field survives the wire, and the
// payload still validates against the shipped schema on the far side.
func TestBindRoundTripsTheMergedShapes(t *testing.T) {
	t.Run("BindInput: need, provided_by, deployment", func(t *testing.T) {
		in := goodBindInput()
		raw := mustJSON(t, in)
		got := decodeAs[spec.BindInput](t, carry(t, raw,
			func(b []byte) proto.Message { return &pb.BindRequest{InputJson: b} },
			func(m proto.Message) []byte { return m.(*pb.BindRequest).GetInputJson() }))
		if got.Need.Type != in.Need.Type || got.Need.Name != in.Need.Name {
			t.Fatalf("need did not survive: got %+v, want %+v", got.Need, in.Need)
		}
		if got.Need.Config["vendor"] != "nvidia" {
			t.Fatalf("the need's opaque config did not survive: %+v", got.Need.Config)
		}
		if got.Provided_by != in.Provided_by {
			t.Fatalf("provided_by did not survive: %q, want %q", got.Provided_by, in.Provided_by)
		}
		if got.Deployment != in.Deployment {
			t.Fatalf("deployment did not survive: %q, want %q", got.Deployment, in.Deployment)
		}
		if err := validateAgainst(t, "#BindInput", raw); err != nil {
			t.Fatalf("the round-tripped payload no longer validates against #BindInput: %v", err)
		}
	})

	t.Run("BindOutput: the binding handle", func(t *testing.T) {
		out := spec.BindOutput{Binding: spec.Opaque{"path": "/run/secrets/gpu-nvidia"}}
		raw := mustJSON(t, out)
		got := decodeAs[spec.BindOutput](t, carry(t, raw,
			func(b []byte) proto.Message { return &pb.BindReply{OutputJson: b} },
			func(m proto.Message) []byte { return m.(*pb.BindReply).GetOutputJson() }))
		if got.Binding["path"] != "/run/secrets/gpu-nvidia" {
			t.Fatalf("the binding handle did not survive: %+v", got.Binding)
		}
		if err := validateAgainst(t, "#BindOutput", raw); err != nil {
			t.Fatalf("the round-tripped binding no longer validates against #BindOutput: %v", err)
		}
	})

	t.Run("UnbindInput: the same handle", func(t *testing.T) {
		in := spec.UnbindInput{Binding: spec.Opaque{"path": "/run/secrets/gpu-nvidia"}}
		raw := mustJSON(t, in)
		got := decodeAs[spec.UnbindInput](t, carry(t, raw,
			func(b []byte) proto.Message { return &pb.UnbindRequest{InputJson: b} },
			func(m proto.Message) []byte { return m.(*pb.UnbindRequest).GetInputJson() }))
		if got.Binding["path"] != "/run/secrets/gpu-nvidia" {
			t.Fatalf("the unbind handle did not survive: %+v", got.Binding)
		}
		if err := validateAgainst(t, "#UnbindInput", raw); err != nil {
			t.Fatalf("the round-tripped unbind input no longer validates against #UnbindInput: %v", err)
		}
	})
}

// TestBindInputIsValidatedAtTheBoundary is the ABSENCE half: a payload the schema refuses must not
// pass the boundary merely because the bytes arrived intact. A `need.type` outside #Word, and a
// `provided_by` that is not a #Ref, are both refused — the wire transports, the schema decides.
func TestBindInputIsValidatedAtTheBoundary(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"a need type outside #Word", `{"need":{"type":"Not A Word","name":"x","config":{}},"provided_by":"podman","deployment":"bed"}`},
		{"a provided_by that is not a #Ref", `{"need":{"type":"gpu","name":"x","config":{}},"provided_by":"Podman Driver","deployment":"bed"}`},
		{"a missing required field", `{"need":{"type":"gpu","name":"x","config":{}},"deployment":"bed"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The bytes travel fine — the rejection is the schema's, not the transport's.
			wire, err := proto.Marshal(&pb.BindRequest{InputJson: []byte(c.raw)})
			if err != nil {
				t.Fatalf("marshal BindRequest: %v", err)
			}
			var back pb.BindRequest
			if err := proto.Unmarshal(wire, &back); err != nil {
				t.Fatalf("unmarshal BindRequest: %v", err)
			}
			if got := string(back.GetInputJson()); got != c.raw {
				t.Fatalf("the bytes did not survive transport: %q", got)
			}
			if err := validateAgainst(t, "#BindInput", back.GetInputJson()); err == nil {
				t.Fatal("the boundary accepted a #BindInput the schema refuses — the payload is transported, not validated")
			}
		})
	}
	// And the positive control, so a validate-always-fails helper cannot pass this test.
	if err := validateAgainst(t, "#BindInput", mustJSON(t, goodBindInput())); err != nil {
		t.Fatalf("a well-formed #BindInput must validate: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
