package spec

// workflow_arg_list_wire_test.go — the WIRE half of the runtime-list-arg change: the
// concrete fixture validates against the shipped schema, and a list survives the JSON
// the two hops in the chain actually use.
//
// WHY THIS IS NOT A DECODE-ONLY ROUND-TRIP. A JSON round-trip alone cannot prove the
// schema admits the document — a Go struct with no CUE behind it round-trips anything
// its annotations allow (the reasoning pipeline_test.go states for closedness). So the
// first test below validates the fixture as CUE, against the SAME concatenation the
// runtime gate and `charly task cue-gen` use; the round-trip then proves the shape the
// schema just admitted is the shape the generated types carry end to end.
//
// The fixture lives in `schema/testdata/` because `schema/schema.go` embeds `*.cue` at
// the FS root only: it is deliberately NOT part of the shipped schema, and this test
// reads it from disk and concatenates it, exactly as the `cue vet` run in the PR body
// does. One fixture, two consumers (this test and that command) — not two copies.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schemaconcat"
)

// The generated fields MUST be `map[string]any`. A `map[string]string` cannot hold a
// list at all, and these three lines turn a reverted `@go(,type=...)` annotation into a
// COMPILE error instead of a silent contract regression discovered by a consumer three
// legs downstream.
var (
	_ map[string]any = WorkflowRunRequest{}.Args
	_ map[string]any = WorkflowSchedule{}.Args
	_ map[string]any = PipelineSchedule{}.Args
)

// loadSchemaWithFixture compiles the shipped schema concatenation TOGETHER with one
// concrete document from schema/testdata, which is how the fixture is validated: the
// document is concatenated (no package clause, like every schema/*.cue file) so the defs
// it asserts against are the real ones.
func loadSchemaWithFixture(t *testing.T, fixture string) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(os.DirFS(".."), "schema", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	body, err := os.ReadFile(filepath.Join("..", "schema", "testdata", fixture))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	v := cuecontext.New().CompileString(src + "\n" + string(body))
	if err := v.Err(); err != nil {
		t.Fatalf("compile schema + %s: %v", fixture, err)
	}
	return v
}

// TestWorkflowArgListFixtureValidates pins the document the chain's lane primitive
// sends: `--prs a b c` as a LIST, alongside the scalar arg that must keep working, and
// the same values through both schedule defs. Each field the fixture asserts is checked
// as a FINAL concrete value, so a disjunction that stopped resolving (or a def that
// started admitting nothing) fails here rather than at a consumer.
func TestWorkflowArgListFixtureValidates(t *testing.T) {
	schema := loadSchemaWithFixture(t, "workflow_run_list_arg.cue")

	for _, path := range []string{
		"workflowRunListArgRequest",
		"workflowScheduleListArg",
		"workflowTriggerListArg",
	} {
		t.Run(path, func(t *testing.T) {
			val := schema.LookupPath(cue.ParsePath(path))
			if err := val.Err(); err != nil {
				t.Fatalf("%s is not in the fixture: %v", path, err)
			}
			if err := val.Validate(cue.Concrete(false), cue.Final()); err != nil {
				t.Errorf("%s does not validate against the shipped schema: %v", path, err)
			}
		})
	}
}

// TestWorkflowRunRequestListArgSurvivesTheWire proves the generated type carries a list
// across the request JSON — the `--request-json` payload the front-end hands an engine.
// The assertion is on the DECODED TYPE, not on the bytes: `["101","102"]` arriving as the
// string `"[\"101\",\"102\"]"` would still be valid JSON and is exactly the pre-joining
// workaround this change removes.
func TestWorkflowRunRequestListArgSurvivesTheWire(t *testing.T) {
	in := WorkflowRunRequest{
		Pipeline: "lane",
		Args: map[string]any{
			"prs": []any{"101", "102", "103"},
			"who": "world",
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal the request: %v", err)
	}

	// The WIRE SPELLING is unchanged: one JSON object mapping an arg name to its value.
	// A pre-existing consumer that only reads the JSON sees the shape it always did.
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("the request is not valid JSON: %v", err)
	}
	args, ok := envelope["args"].(map[string]any)
	if !ok {
		t.Fatalf("args is not a JSON object on the wire: %s", raw)
	}
	if _, flattened := args["prs"].(string); flattened {
		t.Fatalf("the list was flattened to a string on the wire: %s", raw)
	}

	var back WorkflowRunRequest
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal the request: %v", err)
	}
	got, ok := back.Args["prs"].([]any)
	if !ok {
		t.Fatalf("the list came back as %T, want []any: %s", back.Args["prs"], raw)
	}
	if want := []any{"101", "102", "103"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the list came back as %#v, want %#v", got, want)
	}
	if back.Args["who"] != "world" {
		t.Errorf("the scalar arg came back as %#v, want \"world\"", back.Args["who"])
	}
}

// TestWorkflowTriggerListArgSurvivesTheDescriptorJSON covers the SECOND hop: the
// front-end writes a scheduled pipeline's triggers into `schedule.json` as
// `#WorkflowTrigger` and the engine reads them back to build the timer's `--args-json`.
// That file is the only thing connecting an authored trigger to a scheduled run, so a
// list that survives the request but not the descriptor would make the lane work on a
// manual run and silently change behaviour once scheduled.
func TestWorkflowTriggerListArgSurvivesTheDescriptorJSON(t *testing.T) {
	// The front-end's own descriptor wrapper (plugin-pipeline writes this shape).
	desc := struct {
		Pipeline string            `json:"pipeline"`
		Triggers []WorkflowTrigger `json:"triggers"`
	}{
		Pipeline: "lane",
		Triggers: []WorkflowTrigger{{
			Manual: true,
			Schedule: WorkflowSchedule{
				Cron: "0 * * * *",
				Args: map[string]any{"prs": []any{"101", "102"}},
			},
		}},
	}

	raw, err := json.Marshal(desc)
	if err != nil {
		t.Fatalf("marshal the descriptor: %v", err)
	}
	var back struct {
		Pipeline string            `json:"pipeline"`
		Triggers []WorkflowTrigger `json:"triggers"`
	}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal the descriptor: %v", err)
	}
	if len(back.Triggers) != 1 {
		t.Fatalf("descriptor carried %d triggers, want 1", len(back.Triggers))
	}
	got, ok := back.Triggers[0].Schedule.Args["prs"].([]any)
	if !ok {
		t.Fatalf("the scheduled list arg came back as %T, want []any: %s", back.Triggers[0].Schedule.Args["prs"], raw)
	}
	if want := []any{"101", "102"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the scheduled list arg came back as %#v, want %#v", got, want)
	}
}
