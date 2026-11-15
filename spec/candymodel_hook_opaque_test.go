package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestCandyModelHookIsOpaqueRawJSON pins the two claims the IR-opacity change makes, and fails if
// either is reverted.
//
// 1. THE FIELD IS OPAQUE. `#CandyModel.hook` is `bytes`, so the authored `#CandyHook` no longer has
// to live in `spec` for the model to compile — that is what lets the def move to the plugin that
// owns the candy body (DESIGN.md 3.1). A revert to `*CandyHook` is the regression this catches.
//
// 2. IT SERIALISES AS RAW JSON, NOT BASE64. The whole reason this shape is safe to adopt is that the
// OCI label `ai.opencharly.hook` is ALREADY JSON (authored YAML -> struct -> Marshal -> label ->
// Unmarshal). If the field serialised as base64 the label wire would change silently, and every
// existing reader — spec/container's box_metadata_coneb decode, plugin-pod's PreRemove guard — would
// start seeing garbage instead of a hook. RawBody is json.RawMessage, which encoding/json writes
// inline; this test is what holds that.
func TestCandyModelHookIsOpaqueRawJSON(t *testing.T) {
	field, ok := reflect.TypeOf(CandyModel{}).FieldByName("Hook")
	if !ok {
		t.Fatal("CandyModel has no Hook field — the IR field was renamed or removed")
	}
	if got, want := field.Type, reflect.TypeOf(RawBody(nil)); got != want {
		t.Fatalf("CandyModel.Hook is %v, want %v — the IR field must stay opaque", got, want)
	}

	const authored = `{"post_enable":"echo up","pre_remove":"echo down"}`
	m := CandyModel{Hook: RawBody(authored)}

	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal CandyModel: %v", err)
	}

	var round map[string]json.RawMessage
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("unmarshal the marshalled model: %v", err)
	}
	hook, ok := round["hook"]
	if !ok {
		t.Fatal(`the marshalled model carries no "hook" key`)
	}

	// The bytes on the wire must be the AUTHORED JSON itself — not a quoted string, and not base64.
	if string(hook) != authored {
		t.Fatalf("hook marshalled as %s, want the authored JSON verbatim (%s) — RawBody must serialise inline", hook, authored)
	}
	if len(hook) > 0 && hook[0] == '"' {
		t.Fatalf("hook marshalled as a JSON STRING (%s) — the field lost its RawBody type and became []byte, which encoding/json base64-encodes", hook)
	}
}

// TestCandyModelExtractIsOpaqueRawJSON is the same pair of claims for the SECOND field to go
// opaque: the model must not carry `[]CandyExtract` (or the def could not leave the contract), and
// the bytes must marshal INLINE as the authored JSON array — not a quoted string, not base64.
func TestCandyModelExtractIsOpaqueRawJSON(t *testing.T) {
	field, ok := reflect.TypeOf(CandyModel{}).FieldByName("Extract")
	if !ok {
		t.Fatal("CandyModel has no Extract field")
	}
	if got, want := field.Type, reflect.TypeOf(RawBody(nil)); got != want {
		t.Fatalf("CandyModel.Extract is %v, want %v — the IR field must stay opaque", got, want)
	}

	const authored = `[{"source":"builder","path":"/out/x","dest":"/usr/x"}]`
	raw, err := json.Marshal(CandyModel{Extract: RawBody(authored)})
	if err != nil {
		t.Fatalf("marshal CandyModel: %v", err)
	}
	var round map[string]json.RawMessage
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("unmarshal the marshalled model: %v", err)
	}
	if got := string(round["extract"]); got != authored {
		t.Fatalf("extract marshalled as %s, want the authored JSON verbatim (%s)", got, authored)
	}
}

// TestCandyModelDataIsOpaqueRawJSON is the data twin: the field must not carry []CandyData, and its
// bytes must marshal INLINE as the authored JSON array.
func TestCandyModelDataIsOpaqueRawJSON(t *testing.T) {
	field, ok := reflect.TypeOf(CandyModel{}).FieldByName("Data")
	if !ok {
		t.Fatal("CandyModel has no Data field")
	}
	if got, want := field.Type, reflect.TypeOf(RawBody(nil)); got != want {
		t.Fatalf("CandyModel.Data is %v, want %v — the IR field must stay opaque", got, want)
	}

	const authored = `[{"volume":"workspace","src":"/seed","dest":"/data"}]`
	raw, err := json.Marshal(CandyModel{Data: RawBody(authored)})
	if err != nil {
		t.Fatalf("marshal CandyModel: %v", err)
	}
	var round map[string]json.RawMessage
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("unmarshal the marshalled model: %v", err)
	}
	if got := string(round["data"]); got != authored {
		t.Fatalf("data marshalled as %s, want the authored JSON verbatim (%s)", got, authored)
	}
}
