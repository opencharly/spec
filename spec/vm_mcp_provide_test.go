package spec

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schemaconcat"
)

// loadSchema compiles the SAME concatenation the runtime and `charly task cue-gen` use
// (schemaconcat.ConcatSchema — R3: one concatenation contract), so the CUE-level
// assertions below exercise the schema as it actually ships, not a hand-written
// excerpt (same helper as libvirt_gpu_vocab_cue_test.go).
func loadSchema(t *testing.T) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(os.DirFS(".."), "schema", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return v
}

// TestCheckEnv_MCPProvideJSONRoundTrip is the P4 host-threading gate: the check env's
// mcp_provide must survive a JSON round trip so the out-of-process mcp: check verb can
// resolve the endpoint for VM/host venues (no OCI label). Fails if the field is
// reverted — the json tag would be absent and the entry silently dropped.
func TestCheckEnv_MCPProvideJSONRoundTrip(t *testing.T) {
	env := CheckEnv{
		Box:       "cachyos-vm",
		Venue:     "vm",
		VenueKind: "vm",
		MCPProvide: []CandyMCPProvide{
			{Name: "charly-mcp", URL: "http://127.0.0.1:18765/mcp"},
		},
	}
	data, err := json.Marshal(&env)
	if err != nil {
		t.Fatalf("marshalling CheckEnv: %v", err)
	}
	if !strings.Contains(string(data), "mcp_provide") {
		t.Fatalf("CheckEnv JSON lacks mcp_provide key: %s", data)
	}
	var back CheckEnv
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshalling CheckEnv: %v", err)
	}
	if len(back.MCPProvide) != 1 || back.MCPProvide[0].Name != "charly-mcp" ||
		back.MCPProvide[0].URL != "http://127.0.0.1:18765/mcp" {
		t.Fatalf("mcp_provide did not survive the round trip: got %+v (the schema field is missing?)", back.MCPProvide)
	}
}
