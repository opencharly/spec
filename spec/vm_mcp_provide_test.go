package spec

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"gopkg.in/yaml.v3"

	"github.com/opencharly/spec/schemaconcat"
)

// loadSchema compiles the SAME concatenation the runtime and `task cue:gen` use
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

// TestVm_MCPProvideSchemaParse is the P4 substrate-neutral gate at the SCHEMA level:
// #Vm is CLOSED (an unknown key is a typo), so a kind:vm body carrying mcp_provide
// only validates because the schema field exists — this test FAILS if the field is
// reverted. The entry shape is #CandyMCPProvide (name/url/transport), the same shape
// a box/candy declares, so the reject case (missing required url) also proves the
// field binds to that def and is not a loose passthrough.
//
// Unlike the closedness reject cases in libvirt_gpu_vocab_cue_test.go (conflicts at
// the enum level, caught with Concrete(false)), the missing-url case is an
// INCOMPLETENESS — after unification the entry evaluates to `url: !=""`, which
// Concrete(false) allows. The complete-instance judgement (the same one an authored
// charly.yml must satisfy) needs Concrete(true).
func TestVm_MCPProvideSchemaParse(t *testing.T) {
	schema := loadSchema(t)
	def := schema.LookupPath(cue.ParsePath("#Vm"))
	if err := def.Err(); err != nil {
		t.Fatalf("lookup #Vm: %v", err)
	}

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		{
			name:  "kind:vm charly.yml with mcp_provide parses",
			value: `{source: {kind: "cloud_image", url: "https://example.com/os.qcow2"}, mcp_provide: [{name: "charly-mcp", url: "http://127.0.0.1:18765/mcp"}, {name: "charly-sse", url: "http://127.0.0.1:18766/sse", transport: "sse"}]}`,
		},
		{
			name:  "mcp_provide without transport defaults to http",
			value: `{source: {kind: "cloud_image", url: "https://example.com/os.qcow2"}, mcp_provide: [{name: "charly-mcp", url: "http://127.0.0.1:18765/mcp"}]}`,
		},
		{
			// #CandyMCPProvide requires url — proves the field binds the candy
			// shape rather than an open passthrough.
			name:   "mcp_provide entry missing url rejected",
			value:  `{source: {kind: "cloud_image", url: "https://example.com/os.qcow2"}, mcp_provide: [{name: "charly-mcp"}]}`,
			reject: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			val := schema.Context().CompileString(tc.value)
			if err := val.Err(); err != nil {
				t.Fatalf("compile value: %v", err)
			}
			got := def.Unify(val).Validate(cue.Concrete(true), cue.Final())
			if tc.reject && got == nil {
				t.Errorf("value ACCEPTED, want rejection: %s", tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("value rejected: %s — %v", tc.value, got)
			}
			if tc.reject && got != nil && !strings.Contains(got.Error(), "url") {
				t.Logf("note: rejection message does not name the url field: %v", got)
			}
		})
	}
}

// TestVm_MCPProvideYAMLRoundTrip proves the wire side: a kind:vm body authored in a
// charly.yml with mcp_provide survives a marshal/unmarshal round trip (the R10 gate
// for the field — it FAILS without the schema field, since the yaml tag would be
// absent and the entry silently dropped).
func TestVm_MCPProvideYAMLRoundTrip(t *testing.T) {
	body := `source:
  kind: cloud_image
  url: https://example.com/os.qcow2
mcp_provide:
  - name: charly-mcp
    url: http://127.0.0.1:18765/mcp
`
	var vm Vm
	if err := yaml.Unmarshal([]byte(body), &vm); err != nil {
		t.Fatalf("unmarshalling kind:vm body: %v", err)
	}
	if len(vm.MCPProvide) != 1 {
		t.Fatalf("mcp_provide did not parse: got %+v", vm.MCPProvide)
	}
	if vm.MCPProvide[0].Name != "charly-mcp" || vm.MCPProvide[0].URL != "http://127.0.0.1:18765/mcp" {
		t.Fatalf("mcp_provide entry parsed wrong: %+v", vm.MCPProvide[0])
	}
	// Re-marshal and confirm the field survives yaml -> Go -> yaml.
	out, err := yaml.Marshal(&vm)
	if err != nil {
		t.Fatalf("re-marshalling: %v", err)
	}
	var back Vm
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshalling re-marshalled body: %v", err)
	}
	if len(back.MCPProvide) != 1 || back.MCPProvide[0].Name != "charly-mcp" {
		t.Fatalf("mcp_provide did not survive the round trip: got %+v (the schema field is missing?)", back.MCPProvide)
	}
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
