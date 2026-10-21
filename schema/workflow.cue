// CUE schema for the workflow-ENGINE WIRE: the op envelopes a pipeline engine is
// invoked with, plus the approval / schedule / trigger value objects those envelopes
// and an engine's own replies are built from.
//
// The authored form is #Pipeline (schema/pipeline.cue), and #Pipeline.engine selects the
// engine, which is dispatched over the normal InvokeProvider path as
// `InvokeProvider("workflow", <engine>, <op>)` — the `workflow` PROVIDER CLASS declared in
// #ProviderClassNames, so an engine is addressable exactly the way every other plugin is.
// Normalizing a pipeline into the form an engine runs (lobster's
// `.opencharly/pipelines/<name>/workflow.lobster`, a future engine's
// `.github/workflows/<name>.yml`) belongs to THAT engine, not to this contract, which
// carries only the wire the dispatch crosses. `command:lobster` is a SEPARATE, additional
// face of the same engine plugin (its CLI), never the dispatch that runs a workflow. The
// op envelopes below are the `--request-json` payload shapes that path carries.
//
// The former contract-level normalized-IR defs were RETIRED (opencharly/spec#192):
// nothing in the org produced or consumed a lowered IR document — the engines consume the
// envelopes below and carry these value objects themselves — so the defs and the
// narrative that made them read as the live engine wire were deleted together.
// #WorkflowApproval, #WorkflowStepResult, #WorkflowSchedule, #WorkflowTrigger and every
// envelope an engine actually uses remain, unchanged.
//
// CLOSED. A consumer that cannot express a field MUST fail hard rather than
// silently drop it (an engine silently ignoring `approval` would run a workflow a
// human never approved).

// #WorkflowApproval — the object form of the approval gate; the IR always carries
// this shape (the authored `approval: true` / `approval: "msg"` union is normalized
// to it before the IR exists).
//
// Everything below `timeout_ms` is the ENGINE's own approval envelope, which the
// reply could not carry before: an engine computes these (from the gate's config, the
// paused step's own `requiresApproval` JSON, or the environment) and a caller needs
// them BOTH to present the gate and to answer it by its short id. They are IR-only —
// never authored — so carrying them is not a charly.yml format change.
#WorkflowApproval: {
	message?:    string & !=""
	timeout_ms?: int & >0 @go(TimeoutMs)

	// The SHORT id `#WorkflowResumeRequest.id` accepts in place of the resume token.
	// An engine that issues none omits it.
	approval_id?: string & !="" @go(ApprovalID)
	// What is being approved, as the engine renders it — the SAME items the tool
	// envelope shows a human, verbatim.
	items?:   [..._] @go(Items,type=[]any)
	preview?: string & !=""
	// The approver-identity policy the gate enforces, echoed so a caller can show WHO
	// may approve before anyone tries (an engine enforces it at resume time).
	initiated_by?:               string & !="" @go(InitiatedBy)
	required_approver?:          string & !="" @go(RequiredApprover)
	require_different_approver?: bool @go(RequireDifferentApprover)
}

// #WorkflowArgVal — the VALUE one workflow arg may carry: a scalar, or a FLAT
// sequence of scalars.
//
// WHY A SEQUENCE IS ADMITTED AT ALL. An arg map is the ONE channel a runtime value
// reaches an engine through, and a real workflow needs a COLLECTION as often as a
// single value — the retired lane model's `--prs a b c` is the canonical case.
// While every arg map on this wire was `map[string]string`, such a list could not
// cross it: a caller had to smuggle it as one pre-joined string for the callee to
// re-split, which no generic consumer can do correctly. The engine's own arg
// resolution already carries `map[string]any` (`resolveWorkflowArgs` overlays a
// `#LobsterFile` arg default, whose type is `_`), so the sequence was already
// being carried on BOTH sides of a wire that refused it in the middle.
//
// The bounds are DELIBERATE: a scalar, or a flat list of scalars — never a nested
// map and never a list of lists. An arg is addressed by name and rendered into a
// per-arg binding (a CLI `-p`, an env var, or the `LOBSTER_ARGS_JSON` document); a
// nested structure has no such rendering, so admitting one would let a document
// VALIDATE and then be dropped or fail at execution time — the silent-drop failure
// this file's header forbids. Anything richer than a flat list belongs in the
// step's own data, not in an arg.
//
// The Go type at each use is `map[string]any` (the field-level `@go(,type=...)`
// override), because a scalar-or-sequence disjunction has no single Go spelling.
// Widening it also closes a pre-existing divergence: the CUE gate already admitted
// `number` and `bool` here while the generated field was `string`, so a document
// could validate and then fail to decode.
//
// The NAME matters, for the reason the NOTE further down gives: schemagen derives
// `spec.KindValueDefs` from defs matching `#<Word>Value`, so `#WorkflowArgVal`
// (like the shared `#StrVal`) registers no kind. `#WorkflowArgValue` would.
#WorkflowArgVal: #StrVal | [...#StrVal] @go(-)

// #WorkflowSchedule — the normalized cron trigger (see #PipelineSchedule).
#WorkflowSchedule: {
	cron!:       string & !=""
	timezone?:   string & !=""
	args?:       {[string]: #WorkflowArgVal} @go(Args,type=map[string]any)
	persistent?: bool
}

// #WorkflowTrigger — how a workflow starts.
#WorkflowTrigger: {
	manual?:   bool
	schedule?: #WorkflowSchedule
}

// #WorkflowStepResult — the OUTCOME of one step. Recorded in the run state (and in
// a schedule/approval record), never authored. `output` is the step's decoded
// stdout: a charly step's `--output` body, or a shell step's auto-parsed JSON
// (lobster's `$id.json`), so a later `when` can address it by dotted path.
//
// It SURVIVES the IR retirement below: `#WorkflowRunReply.steps` carries it, so it is
// live wire an engine's reply is built from.
#WorkflowStepResult: {
	id!:         string & !=""
	status!:     "ok" | "skipped" | "failed" | "cancelled" | "needs_approval" | "needs_input"
	exit_code?:  int @go(ExitCode,type=*int)
	stdout?:     string
	stderr?:     string
	output?:     _ @go(Output,type=any)
	duration_ms?: int & >=0 @go(DurationMs)
	attempts?:   int & >=0
}

// NOTE — there is deliberately NO #WorkflowValue here.
//
// Every #<X>Value def in schema/ is the host-side value gate for an AUTHORABLE
// ENTITY KIND (candy, local, pod, vm, task, pipeline, …), and schemagen DERIVES
// spec.KindValueDefs from their names. A #WorkflowValue would therefore register
// `workflow` as a KIND — and nothing serves `kind: workflow`: the kinds are
// plugin-served (#Node is an open struct and the arm-derived KindWords is empty), and
// plugin-lobster serves the `workflow` PROVIDER CLASS, not a kind. A registered kind
// no plugin serves would let the host gate a `kind: workflow` node against a def for
// a kind that does not exist. The engine wire below is not a kind: it is what an engine
// is dispatched with and answers over, never authored in a charly.yml.
//
// `spec/spec/pipeline_test.go` pins this structurally (KindValueDefs and
// ProviderClasses must stay DISJOINT — a word that is both is exactly this bug).

// ---------------------------------------------------------------------------
// Engine op envelopes. Dispatched as InvokeProvider("workflow", <engine>, <op>)
// payloads over the `workflow` provider class (`--request-json`); the reply shapes are
// lobster's tool-mode envelope v1, so an engine's output is interchangeable with
// upstream lobster's.
// ---------------------------------------------------------------------------

// #WorkflowRunRequest — start a workflow.
//
// `args` carries the run's RUNTIME arg values, so it admits `#WorkflowArgVal` (a
// scalar, or a flat sequence of scalars) rather than the scalar-only `#StrVal`: a
// workflow that operates on a list — the retired lane model's `--prs a b c` — has
// no other channel to receive one. The WIRE is unchanged: still one JSON object
// mapping arg name to value, and every payload that validated before still
// validates, because the admitted value set only GREW. What changes is the
// generated Go field, `map[string]string` → `map[string]any`, which is a
// compile-time change for a Go caller that BUILDS the request and no change at all
// for anything that only reads the JSON.
#WorkflowRunRequest: {
	pipeline!: string & !=""
	args?:     {[string]: #WorkflowArgVal} @go(Args,type=map[string]any)
	mode?:     *"human" | "tool"
	dry_run?:  bool @go(DryRun)
	gen_dir?:  string & !="" @go(GenDir)
}

// #WorkflowRunReply — lobster envelope v1. `resume_token` is present exactly when
// status is needs_approval/needs_input and is wire-compatible with upstream lobster.
#WorkflowRunReply: {
	status!:            "ok" | "needs_approval" | "needs_input" | "cancelled" | "error"
	output?:            string
	requires_approval?: #WorkflowApproval @go(RequiresApproval)
	requires_input?:    #WorkflowInputRequest @go(RequiresInput)
	resume_token?:      string & !="" @go(ResumeToken)
	cost?:              number & >=0
	error?:             string
	steps?:             [...#WorkflowStepResult]
}

// #WorkflowInputRequest — the pending `input` gate a needs_input reply describes.
#WorkflowInputRequest: {
	step!:            string & !=""
	prompt!:          string & !=""
	response_schema?: {[string]: _} @go(ResponseSchema,type=map[string]any)
	defaults?:        {[string]: #StrVal} @go(Defaults,type=map[string]string)
	// The SUBJECT the gate was shown — the engine's own resolved value (the gate's
	// `stdin`, else the previous step's output), so a caller can render what the
	// question is about without re-deriving it. IR-only, like the approval fields.
	subject?: _ @go(Subject,type=any)
}

// #WorkflowResumeRequest — answer a pending approval/input gate. `token` resumes by
// resume token; `id` resumes by the short approval id. Exactly one is set (Go).
//
// `approve` is TRI-STATE, and the pointer spelling below is the whole point of it. A
// plain `bool` carrying `omitempty` makes `false` byte-identical to an ABSENT field,
// so "the human rejected this" and "no approval answer was given at all" collapse into
// the same wire value — a rejected gate is indistinguishable from an untouched one,
// and the approval half of a resume is unreachable. `*bool` (the recipe's pointer /
// tri-state arm) separates the three states on the wire: absent = no answer,
// `false` = rejected, `true` = approved. It is an ANNOTATION, not a type change — the
// CUE type stays `bool` and the wire key stays `approve`, so there is no authored
// wire-key change and no version machinery to touch (see AGENTS.md, "Modify this repo").
//
// `cancel` is NOT the rejection arm and must not be read as one: it ABORTS the gate
// without judging the proposal (upstream lobster's tool-mode `cancel`), so a caller
// that maps "no" onto it loses the distinction this field exists to carry.
#WorkflowResumeRequest: {
	pipeline?:     string & !=""
	token?:        string & !="" @go(Token)
	id?:           string & !=""
	approve?:      bool @go(,type=*bool)
	response?:     {[string]: _} @go(Response,type=map[string]any)
	cancel?:       bool
}

// #WorkflowScheduleRequest — the systemd-user-timer scheduler surface.
#WorkflowScheduleRequest: {
	op!:       "apply" | "list" | "remove" | "run-now"
	pipeline?: string & !=""
}

// #WorkflowScheduleEntry — one installed timer. `next_run` is the engine's own
// computed next fire time; `on_calendar` is the systemd expression it installed.
#WorkflowScheduleEntry: {
	pipeline!:    string & !=""
	timer!:       string & !=""
	on_calendar!: string & !="" @go(OnCalendar)
	active!:      bool
	next_run?:    string & !="" @go(NextRun)
}

// #WorkflowScheduleReply — the scheduler answer.
#WorkflowScheduleReply: {
	units?:   [...string]
	entries?: [...#WorkflowScheduleEntry]
}

// #WorkflowEmitRequest — lower the IR to a consumer's on-disk form. `format` selects
// the consumer(s): `lobster` and `charly-yml` are the lobster engine's pair;
// `github-actions` is the designed-but-unbuilt consumer.
#WorkflowEmitRequest: {
	pipeline!: string & !=""
	format!:   [...("lobster" | "charly-yml" | "github-actions")] @go(Format)
	out_dir?:  string & !="" @go(OutDir)
}

// #WorkflowEmitReply — the absolute paths written, one per emitted file.
#WorkflowEmitReply: {
	files!: {[string]: string}
}

// #WorkflowEngineCapability — the static facts an engine answers (OpDescribe), so a
// caller can refuse an unsupported feature with a clear error instead of silently
// dropping it.
#WorkflowEngineCapability: {
	name!:         string & !=""
	execute!:      bool
	resume!:       bool
	approvals!:    bool
	schedule!:     bool
	emit_formats!: [...string] @go(EmitFormats)
}
