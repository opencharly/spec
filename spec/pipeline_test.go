package spec

// pipeline_test.go — the closedness corpus for the `kind: pipeline` AUTHORED body
// (schema/pipeline.cue) and the class-route gate over the workflow vocabulary.
//
// WHY THIS TEST EXISTS (R2, and the reason it is not a decode-only round-trip): the
// closure lives ONLY in CUE. `cue exp gengotypes` renders every one of these fields as a
// plain Go `string` / `[]Step`, so a Go-level round-trip cannot tell a closed body from an
// open one — a test that only decodes VALID documents passes identically with the
// closedness reverted and therefore proves nothing. Every reject case below unifies a
// CONCRETE value into `#PipelineValue` and demands the same judgement the host's value gate
// makes on an authored charly.yml, so it FAILS the moment a field is reopened or a retired
// alias is re-accepted.
//
// The schema is compiled from the SAME concatenation the runtime gate and
// `charly task cue-gen` use (schemaconcat.ConcatSchema — R3, one concatenation contract),
// so this exercises the shipped schema, never a hand-written excerpt of it.

import (
	"os"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schemaconcat"
)

// loadPipelineSchema compiles the runtime's own schema concatenation.
func loadPipelineSchema(t *testing.T) cue.Value {
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

func TestPipelineValueClosedness(t *testing.T) {
	schema := loadPipelineSchema(t)
	def := schema.LookupPath(cue.ParsePath("#PipelineValue"))
	if err := def.Err(); err != nil {
		t.Fatalf("lookup #PipelineValue: %v", err)
	}

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		// --- accept: the lobster form ---
		{
			"shell step accepted",
			`{description: "d", steps: [{id: "s1", run: "echo hi"}]}`,
			false,
		},
		{
			"full flow keys accepted",
			`{description: "d", steps: [{
				id: "s1", run: "echo hi",
				when: "$prev.json.ok == true",
				env: {FOO: "bar", BAR: "${pr}"},
				cwd: "/tmp", stdin: "$prev.stdout",
				timeout_ms: 5000, on_error: "continue",
				retry: {max: 3, delay_ms: 100, factor: 2.0},
				approval: {message: "ok?", timeout_ms: 60000},
			}]}`,
			false,
		},
		{
			"terse approval union accepted",
			`{description: "d", steps: [{id: "s1", run: "echo hi", approval: true}]}`,
			false,
		},
		{
			"parallel accepted",
			`{description: "d", steps: [{id: "s1", parallel: {wait: "any", branches: [
				{id: "b1", run: "echo a"}, {id: "b2", run: "echo b"},
			]}}]}`,
			false,
		},
		{
			"for_each with companions accepted",
			`{description: "d", steps: [{
				id: "s1", for_each: "$prev.json.items",
				item_var: "it", index_var: "i", batch_size: 2, pause_ms: 10,
				steps: [{id: "inner", run: "echo $it"}],
			}]}`,
			false,
		},

		// --- accept: the charly form (the FULL #Step grammar, inside plan:) ---
		{
			"plan with a builtin verb accepted",
			`{description: "d", steps: [{id: "s1", plan: [{run: "echo hi"}, {mkdir: "/tmp/wf"}]}]}`,
			false,
		},
		{
			"plan check intent accepted",
			`{description: "d", steps: [{id: "s1", plan: [{check: "api is up", timeout: "30s"}]}]}`,
			false,
		},
		{
			"plan agent-run intent accepted",
			`{description: "d", steps: [{id: "s1", plan: [{"agent-run": "summarise the diff"}]}]}`,
			false,
		},
		{
			"argv step accepted",
			`{description: "d", steps: [{id: "s1", charly: ["task", "list"]}]}`,
			false,
		},
		{
			"argv step as a bare string accepted",
			`{description: "d", steps: [{id: "s1", charly: "task list"}]}`,
			false,
		},

		// --- accept: the workflow level ---
		{
			"workspace fields + a schedule trigger accepted",
			`{
				description: "d",
				engine: "lobster",
				args: {pr: {description: "the PR", required: true}},
				env: {CI: "1"},
				cwd: "/work",
				cost_limit: 2.5,
				triggers: [{manual: true}, {schedule: {cron: "0 9 * * 1-5", timezone: "UTC", persistent: true, args: {branch: "main"}}}],
				config: {llm: {model: "x"}, redo: {max: 1}},
				entities: {"my-task": {task: {description: "t", plan: [{run: "true"}]}}},
				steps: [{id: "s1", run: "echo hi"}],
			}`,
			false,
		},

		// --- reject: exactly-one exec arm is a GO rule, but the arms must not silently
		//     co-exist in a way the schema blesses ---
		{
			"two lobster exec arms are a Go rule, so CUE must NOT reject them here",
			`{description: "d", steps: [{id: "s1", run: "echo hi", pipeline: "other"}]}`,
			false, // documented: the XOR is OpValidate's job (see the pipeline.cue header)
		},

		// --- reject ---
		{
			"unknown step key rejected",
			`{description: "d", steps: [{id: "s1", run: "echo hi", bogus: 1}]}`,
			true,
		},
		{
			"the retired `stages:` key rejected",
			`{description: "d", stages: [{kind: "command", command: "echo hi"}]}`,
			true,
		},
		{
			"the retired `command:` alias rejected",
			`{description: "d", steps: [{id: "s1", command: "echo hi"}]}`,
			true,
		},
		{
			"the retired `skip_when:` alias rejected",
			`{description: "d", steps: [{id: "s1", run: "echo hi", skip_when: "@x.y == 1"}]}`,
			true,
		},
		{
			"unknown workspace key rejected",
			`{description: "d", concurrency: {lanes: 2}, steps: [{id: "s1", run: "echo hi"}]}`,
			true, // `concurrency:` is CLI/`for_each` semantics now, never an entity key
		},
		{
			"a step with no id rejected",
			`{description: "d", steps: [{run: "echo hi"}]}`,
			true,
		},
		{
			"a step with no exec arm is a Go rule too, so CUE must NOT reject it here either",
			`{description: "d", steps: [{id: "s1", when: "true"}]}`,
			false, // OpValidate rejects both zero arms and two arms; see the pipeline.cue header
		},
		{
			"an empty steps list rejected",
			`{description: "d", steps: []}`,
			true,
		},
		{
			"an empty charly argv rejected",
			`{description: "d", steps: [{id: "s1", charly: []}]}`,
			true,
		},
		{
			"a bad on_error value rejected",
			`{description: "d", steps: [{id: "s1", run: "echo hi", on_error: "explode"}]}`,
			true,
		},
		{
			"a zero timeout_ms rejected",
			`{description: "d", steps: [{id: "s1", run: "echo hi", timeout_ms: 0}]}`,
			true,
		},
		{
			"a schedule trigger with no cron rejected",
			`{description: "d", triggers: [{schedule: {timezone: "UTC"}}], steps: [{id: "s1", run: "echo hi"}]}`,
			true,
		},
		{
			"a parallel with no branches rejected",
			`{description: "d", steps: [{id: "s1", parallel: {}}]}`,
			true,
		},
		{
			"a nested for_each in a substep rejected",
			`{description: "d", steps: [{id: "s1", for_each: "$x", steps: [
				{id: "inner", run: "echo hi", for_each: "$y"},
			]}]}`,
			true, // #PipelineSubStep is non-recursive by construction (lobster's own rule)
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The value must be compiled in the SAME context as the schema; unifying
			// across contexts is not meaningful.
			val := schema.Context().CompileString(tc.value)
			if err := val.Err(); err != nil {
				t.Fatalf("compile value: %v", err)
			}
			got := def.Unify(val).Validate(cue.Concrete(false), cue.Final())

			if tc.reject && got == nil {
				t.Errorf("ACCEPTED a value the schema must reject:\n%s", tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("REJECTED a valid value: %v\n%s", got, tc.value)
			}
		})
	}
}

// TestPipelineClassCoverage is the R2 gate: every provider class in the CLOSED vocabulary
// must have a decided workflow route, and the route table must not name a class that does
// not exist. Both directions matter — a one-way check would let a stale row outlive the
// class it describes. It lives in spec because the table IS spec data; plugin-pipeline
// re-runs it against its lowering in W5.
func TestPipelineClassCoverage(t *testing.T) {
	if len(ProviderClasses) == 0 {
		t.Fatal("ProviderClasses is empty — schemagen did not emit the vocabulary")
	}
	for _, class := range ProviderClasses {
		if PipelineClassRoutes[class] == "" {
			t.Errorf("provider class %q has no workflow route — adding a class MUST force the route decision (spec.PipelineClassRoutes)", class)
		}
	}
	inVocab := make(map[string]bool, len(ProviderClasses))
	for _, class := range ProviderClasses {
		inVocab[class] = true
	}
	for class := range PipelineClassRoutes {
		if !inVocab[class] {
			t.Errorf("PipelineClassRoutes names %q, which is not in ProviderClasses — the table has drifted from the vocabulary", class)
		}
	}
}

// TestKindAndClassVocabulariesAreDisjoint pins the KIND-vs-CLASS split structurally.
//
// KindValueDefs is DERIVED from the `#<X>Value` def names, and each of those defs is the
// host-side value gate for an AUTHORABLE ENTITY KIND — a word some plugin serves as
// `kind: <word>`. ProviderClasses is a DISPATCH vocabulary: the faces a plugin can be
// invoked through. The two are different axes of the same plugin, and a word that appears
// in BOTH is a defect with a specific shape: it makes `kind: <word>` gateable while
// `<word>:` dispatches to a plugin, so the host validates a kind no plugin actually serves.
//
// This is not hypothetical — it is the exact bug this change shipped once and then
// removed: a `#WorkflowValue` def (added by analogy with `#PipelineValue`, without noticing
// that `pipeline` is a plugin-SERVED KIND while `workflow` is a provider CLASS) silently
// registered `workflow` in KindValueDefs. The generated map cannot show that on its own,
// which is why the gate is here rather than in a comment: a decode-only or generation-only
// check passes either way.
func TestKindAndClassVocabulariesAreDisjoint(t *testing.T) {
	if len(KindValueDefs) == 0 || len(ProviderClasses) == 0 {
		t.Fatal("a vocabulary is empty — schemagen did not emit one of them")
	}
	for kind := range KindValueDefs {
		for _, class := range ProviderClasses {
			if kind == class {
				t.Errorf("%q is BOTH a kind (KindValueDefs) and a provider class (ProviderClasses): "+
					"a kind is a word a plugin SERVES as `kind: %s`, a class is a face it is DISPATCHED through — "+
					"a word in both makes `kind: %s` gateable against a def for a kind no plugin serves", kind, kind, kind)
			}
		}
	}
}
