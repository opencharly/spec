package spec

// workflow_envelope_test.go — the closedness corpus for the workflow ENGINE op
// envelopes (schema/workflow.cue): #WorkflowApproval and #WorkflowInputRequest.
//
// WHY THIS TEST EXISTS. An engine's approval envelope is strictly RICHER than the IR
// used to carry: plugin-lobster's `approvalRequest` computes `approvalId`, `items`,
// `preview` and the approver-identity policy, and #WorkflowApproval had a field for
// none of them. So a caller could neither show a human what was being approved nor
// answer the gate by its SHORT id — #WorkflowResumeRequest.id had nothing to be
// resolved against, because the reply never carried the id. `subject` is the same
// story on the input gate: the engine resolves what the question is ABOUT (the gate's
// `stdin`, else the previous step's output) and the reply could not say.
//
// CLOSEDNESS, not a decode round-trip (R2, same reasoning as pipeline_test.go): the
// closure lives ONLY in CUE, so this unifies concrete values into the defs and demands
// the same judgement the host's value gate makes. The last case is the load-bearing
// one — it holds the AUTHORED def (#PipelineApproval) to its UNCHANGED surface, which
// is what makes the whole change IR-only: no authored wire key moved, so — the rule the
// recipe states as "a schema-version bump ONLY on an authored wire-key change" — there
// is no version STAMP to bump. The `#SchemaVersion` / `#SchemaFloor` CalVer, the
// `charly.yml` `version:` stamp and the equality gate were DELETED
// (CHANGELOG/0.2026270.938.md). `charly migrate` (`plugin-migrate`, `command:migrate`)
// was NOT deleted — it was a LEG of that same chain and survives as the live remedy: it
// is what strips a retired `version:` stamp, and its `check-migrate-local` bed is what
// proves it. So a future authored wire-key change still has a route; what it does not
// have is a stamp to bump. That distinction is the one this corpus pins.

import (
	"testing"

	"cuelang.org/go/cue"
)

// unifyDef unifies a concrete value into a named def in the runtime's own schema
// concatenation and reports the resulting error (nil = accepted).
func unifyDef(t *testing.T, schema cue.Value, defName, value string) error {
	t.Helper()
	def := schema.LookupPath(cue.ParsePath(defName))
	if err := def.Err(); err != nil {
		t.Fatalf("lookup %s: %v", defName, err)
	}
	val := schema.Context().CompileString(value)
	if err := val.Err(); err != nil {
		t.Fatalf("compile value: %v", err)
	}
	return def.Unify(val).Validate(cue.Concrete(false), cue.Final())
}

func TestWorkflowApprovalCarriesTheEngineEnvelope(t *testing.T) {
	schema := loadPipelineSchema(t)

	for _, tc := range []struct {
		name   string
		def    string
		value  string
		reject bool
	}{
		// --- accept: the envelope the engine actually produces ---
		{
			"the minimal gate still unifies",
			"#WorkflowApproval",
			`{message: "Deploy to prod?", timeout_ms: 60000}`,
			false,
		},
		{
			// Every field is optional, which is the additive-compatibility claim in one
			// line: a reply written BEFORE this change still unifies.
			"an empty approval unifies (every engine field is optional)",
			"#WorkflowApproval",
			`{}`,
			false,
		},
		{
			"the full engine envelope unifies",
			"#WorkflowApproval",
			`{
				message: "Approve the release?",
				timeout_ms: 60000,
				approval_id: "a1b2c3d4",
				items: [{kind: "pr", number: 762}],
				preview: "diff --stat\n 3 files changed",
				initiated_by: "ci-bot",
				required_approver: "atrawog",
				require_different_approver: true,
			}`,
			false,
		},
		{
			// The point of the whole change: the enriched envelope is reachable through
			// the REPLY, which is the only place a caller can read it from.
			"the enriched envelope is reachable through the run reply",
			"#WorkflowRunReply",
			`{
				status: "needs_approval",
				requires_approval: {approval_id: "a1b2c3d4", items: [1, "two"], preview: "p"},
				resume_token: "v1:workflow-file:abc",
			}`,
			false,
		},
		{
			"the input gate carries its subject",
			"#WorkflowInputRequest",
			`{step: "ask", prompt: "which branch?", subject: {branch: "main", prs: [1, 2]}}`,
			false,
		},
		{
			// Backwards compatibility on the input gate too: a request with no resolved
			// subject is still a legal request.
			"an input gate with no subject unifies",
			"#WorkflowInputRequest",
			`{step: "ask", prompt: "which branch?"}`,
			false,
		},
		{
			"the short id resumes without a token",
			"#WorkflowResumeRequest",
			`{pipeline: "nightly", id: "a1b2c3d4", approve: true}`,
			false,
		},

		// --- reject: closedness still holds on the ENRICHED defs ---
		{
			"an unknown approval key is rejected",
			"#WorkflowApproval",
			`{message: "ok?", approved_by: "someone"}`,
			true,
		},
		{
			"items must be a list",
			"#WorkflowApproval",
			`{items: "not-a-list"}`,
			true,
		},
		{
			"an empty approval_id is rejected",
			"#WorkflowApproval",
			`{approval_id: ""}`,
			true,
		},
		{
			"an input gate still requires a prompt",
			"#WorkflowInputRequest",
			`{step: "ask", subject: "x"}`,
			true,
		},
		{
			"an unknown input-request key is rejected",
			"#WorkflowInputRequest",
			`{step: "ask", prompt: "p", question: "q"}`,
			true,
		},

		// --- reject: the AUTHORED surface is unchanged (the IR-only proof) ---
		{
			// This case is the change's contract. The engine's envelope is IR-only — the
			// authored `approval:` never gains these keys — so the authored def must NOT
			// accept them. If someone later "conveniently" mirrors the enrichment here,
			// that IS an authored wire-key change: every authored document carrying
			// `approval:` stops unifying against the CLOSED schema, so it is a declared
			// cutover with a `charly migrate` route for the consumers — and this test
			// says so first, instead of letting the mirror ship as a "small addition".
			"the authored approval def does not gain the engine fields",
			"#PipelineApproval",
			`{message: "ok?", items: [1], approval_id: "a1b2c3d4"}`,
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := unifyDef(t, schema, tc.def, tc.value)
			if tc.reject && got == nil {
				t.Errorf("%s ACCEPTED a value it must reject:\n%s", tc.def, tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("%s REJECTED a valid value: %v\n%s", tc.def, got, tc.value)
			}
		})
	}
}

// TestWorkflowArgsCarryARuntimeList pins the admission `#WorkflowArgVal` adds to the
// three arg maps on the engine wire, AND the bound it does not cross.
//
// WHY THE WIRE NEEDED IT. An arg map is the only channel a runtime value reaches an
// engine through, and a workflow that operates on a COLLECTION — the retired lane
// model's `--prs a b c` — had no way to receive one: every arg map here was
// scalar-only, so the list had to be pre-joined into a string and re-split by the
// consumer. The engine's own resolution already carries `map[string]any`, so the
// sequence was carried on both sides of a wire that refused it in the middle.
//
// WHY THE BOUND IS PINNED. Admitting arbitrary JSON would be a smaller diff and the
// wrong contract: an arg is rendered into a per-arg binding (a CLI `-p`, an env var,
// the `LOBSTER_ARGS_JSON` document), and a nested value has no such rendering — it
// would VALIDATE here and then be dropped or fail at execution time. So the reject
// cases below are as load-bearing as the accept cases: they stop a later "just use
// `_`" from turning a loud schema error into a silent execution-time drop.
//
// The three defs move together because they are ONE mechanism: an authored
// `#PipelineSchedule.args` is normalized into `#WorkflowSchedule.args`, which is what
// a scheduled run's `#WorkflowRunRequest.args` is built from. A run that accepts a
// list while its schedule does not is the half-fix this corpus exists to prevent.
func TestWorkflowArgsCarryARuntimeList(t *testing.T) {
	schema := loadPipelineSchema(t)

	for _, tc := range []struct {
		name   string
		def    string
		value  string
		reject bool
	}{
		// --- accept: the list crossing the wire ---
		{
			"the run request carries a list arg",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {prs: ["101", "102", "103"], who: "world"}}`,
			false,
		},
		{
			"an empty list arg is a value, not an absence",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {prs: []}}`,
			false,
		},
		{
			"the schedule on the engine wire carries a list arg",
			"#WorkflowSchedule",
			`{cron: "0 * * * *", args: {prs: ["101"]}}`,
			false,
		},
		{
			"the AUTHORED schedule carries a list arg too",
			"#PipelineSchedule",
			`{cron: "0 * * * *", args: {prs: ["101"]}}`,
			false,
		},

		// --- accept: nothing that already worked stops working ---
		{
			// The scalar control: the pre-existing shape, byte for byte.
			"a scalar arg is unchanged",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {who: "world"}}`,
			false,
		},
		{
			// The union did not narrow: `#StrVal` already admitted these, and the
			// generated field was the only thing that could not hold them.
			"the numeric and boolean scalars stay admitted",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {count: 3, dry: true}}`,
			false,
		},
		{
			"a list of mixed scalars is admitted",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {mixed: ["a", 2, true]}}`,
			false,
		},

		// --- reject: the bound holds ---
		{
			"a nested map arg is rejected",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {prs: {nested: "x"}}}`,
			true,
		},
		{
			"a list of lists is rejected",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {prs: [["101"]]}}`,
			true,
		},
		{
			"a list of maps is rejected",
			"#WorkflowRunRequest",
			`{pipeline: "lane", args: {prs: [{number: 101}]}}`,
			true,
		},
		{
			"a nested map arg is rejected on the engine schedule too",
			"#WorkflowSchedule",
			`{cron: "0 * * * *", args: {prs: {nested: "x"}}}`,
			true,
		},
		{
			"a nested map arg is rejected on the authored schedule too",
			"#PipelineSchedule",
			`{cron: "0 * * * *", args: {prs: {nested: "x"}}}`,
			true,
		},

		// --- the sub-workflow step arm: the same mechanism one level down ---
		{
			// A `workflow:` step hands its args to the SAME resolveWorkflowArgs the run
			// request does, so leaving this arm scalar-only would let a lane carry a list
			// at the top level and fail one step down.
			"a sub-workflow step carries a list arg",
			"#PipelineStep",
			`{id: "s1", workflow: "child", workflow_args: {prs: ["101", "102"]}}`,
			false,
		},
		{
			"a sub-workflow step still rejects a nested map arg",
			"#PipelineStep",
			`{id: "s1", workflow: "child", workflow_args: {prs: {nested: "x"}}}`,
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := unifyDef(t, schema, tc.def, tc.value)
			if tc.reject && got == nil {
				t.Errorf("%s ACCEPTED a value it must reject:\n%s", tc.def, tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("%s REJECTED a valid value: %v\n%s", tc.def, got, tc.value)
			}
		})
	}
}
