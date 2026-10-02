// CUE schema for the normalized WORKFLOW IR and the workflow-ENGINE op envelopes.
//
// The authored form is #Pipeline (schema/pipeline.cue). A pipeline plugin resolves
// it to #Workflow — the engine-agnostic IR every consumer lowers from:
//
//   lobster engine        .opencharly/pipelines/<name>/{workflow.lobster, charly.yml}
//   github-actions engine .github/workflows/<name>.yml            (future; same IR)
//
// The IR is deliberately the SAME step shape as the authored form (it embeds
// #PipelineFlow + #PipelineArms), so lowering never has to re-invent the grammar;
// what the IR ADDS is the per-step result record and the engine op envelopes. The
// engine is selected by #Pipeline.engine and dispatched over the normal
// InvokeProvider path as `InvokeProvider("workflow", <engine>, <op>)` — the
// `workflow` PROVIDER CLASS added to #ProviderClassNames by this same change, so an
// engine is addressable exactly the way every other plugin is. `command:lobster` is a
// SEPARATE, additional face of the same engine plugin (its CLI), never the dispatch
// that runs a workflow. The op envelopes below are the `--request-json` payload
// shapes that path carries.
//
// CLOSED. A consumer that cannot express an IR feature MUST fail hard rather than
// silently drop it (an engine silently ignoring `approval` would run a workflow a
// human never approved).

// #WorkflowApproval — the object form of the approval gate; the IR always carries
// this shape (the authored `approval: true` / `approval: "msg"` union is normalized
// to it before the IR exists).
#WorkflowApproval: {
	message?:    string & !=""
	timeout_ms?: int & >0 @go(TimeoutMs)
}

// #WorkflowSchedule — the normalized cron trigger (see #PipelineSchedule).
#WorkflowSchedule: {
	cron!:       string & !=""
	timezone?:   string & !=""
	args?:       {[string]: #StrVal} @go(Args,type=map[string]string)
	persistent?: bool
}

// #WorkflowTrigger — how a workflow starts.
#WorkflowTrigger: {
	manual?:   bool
	schedule?: #WorkflowSchedule
}

// #WorkflowStepResult — the OUTCOME of one step. Recorded in the run state (and in
// a schedule/approval record), never authored. `output` is the step's decoded
// stdout: a charly step's `--json-output` body, or a shell step's auto-parsed JSON
// (lobster's `$id.json`), so a later `when` can address it by dotted path.
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

// #WorkflowStep — one IR step. Same grammar as the authored step, plus the
// normalized approval object and the optional recorded result.
#WorkflowStep: {
	#PipelineFlow
	#PipelineArms

	approval?: #WorkflowApproval
	result?:   #WorkflowStepResult
}

// #Workflow — the normalized, engine-agnostic IR. `entities:` is carried verbatim
// from the authored form so a lowerer can emit the generated charly.yml.
#Workflow: {
	description!: string & !=""
	engine!:      string & !=""
	args?:        {[string]: #TaskParamSpec}
	env?:         {PATH?: _|_, [string]: #StrVal} @go(Env,type=map[string]string)
	cwd?:         string & !=""
	cost_limit?:  number & >=0 @go(CostLimit)
	triggers?:    [...#WorkflowTrigger]
	config?:      {[string]: _} @go(Config,type=map[string]any)
	entities?:    {[string]: {...}} @go(Entities,type=map[string]map[string]any)
	steps!:       [...#WorkflowStep]
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
// a kind that does not exist. The IR is the ENGINE WIRE, not a kind: it is produced by
// lowering and consumed by an engine, never authored in a charly.yml.
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
#WorkflowRunRequest: {
	pipeline!: string & !=""
	args?:     {[string]: #StrVal} @go(Args,type=map[string]string)
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
}

// #WorkflowResumeRequest — answer a pending approval/input gate. `token` resumes by
// resume token; `id` resumes by the short approval id. Exactly one is set (Go).
#WorkflowResumeRequest: {
	pipeline?:     string & !=""
	token?:        string & !="" @go(Token)
	id?:           string & !=""
	approve?:      bool
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
