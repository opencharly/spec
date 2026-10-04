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
// is no version machinery to touch. That rule no longer has a mechanism behind it: the
// `#SchemaVersion` / `#SchemaFloor` CalVer, the `charly.yml` `version:` stamp and the
// equality gate were DELETED (CHANGELOG/0.2026270.938.md), and `plugin-migrate` with
// them. What survives is the distinction, and it is the one this corpus pins.

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
