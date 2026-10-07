// CUE schema for the `pipeline` KIND — the AUTHORED body of a workflow.
//
// A pipeline is authored in LOBSTER syntax: `steps:` carrying `id:` plus one exec
// arm (`run` shell / `pipeline` / `workflow` / `parallel` / `for_each` / `input`),
// with `when`/`stdin`/`retry`/`on_error`/`timeout_ms`/`approval` flow keys. It is
// EXTENDED with charly's OWN step grammar so a workflow can use any charly plugin
// of any provider class:
//
//   plan:  [...#Step]   — the SAME grammar kind:task and a candy plan use: every
//                          builtin verb, every plugin verb via `<word>: <input>`
//                          sugar, every #Op modifier.
//   charly: [<argv>…]   — any command-class plugin, run as `charly -C <gen> <argv>`.
//
// Every exec arm executes through the NORMAL charly core→plugin dispatch (loader →
// kit.RunPlan → VerbResolver → host registry → compiled-in or gRPC plugin); this
// schema special-cases no word, so a plugin released tomorrow is usable with no
// change here.
//
// CLOSED (an unknown key is a typo). Shared defs (#Step/#StrVal/#Duration) come from
// _common.cue; #TaskParamSpec from task.cue. #PipelineValue is the HOST value gate:
// schemagen DERIVES spec.KindValueDefs["pipeline"] from it (the #TaskValue
// precedent), so the host closedness-gates a pipeline body with zero per-kind code.
//
// An engine normalizes this pipeline into whatever form IT runs; schema/workflow.cue
// carries the wire that engine is dispatched with.
//
// Exec-arm exclusivity (exactly one of run/pipeline/workflow/parallel/for_each/
// input/plan/charly) is a GO rule in plugin-pipeline's OpValidate, NOT a CUE
// disjunction: an entity-level nested disjunction breaks the closedness of the
// surrounding def and collapses the generated Go type (the box.cue base⊻from
// precedent, which has the same shape).

// #PipelineRetry — lobster `retry`: retry the failed step up to `max` attempts.
#PipelineRetry: {
	max?:      int & >0
	delay_ms?: int & >=0 @go(DelayMs)
	factor?:   number & >=1
}

// #PipelineApproval — the OBJECT form of lobster `approval`. The AUTHORED form is a
// union (bool | string | this struct) so `approval: true` stays terse; an engine
// normalizes it to this object form before it evaluates the gate.
#PipelineApproval: {
	message?:    string & !=""
	timeout_ms?: int & >0 @go(TimeoutMs)
}

// #PipelineFlow — the flow keys EVERY pipeline step carries, whatever its exec arm.
// Embedded by each step def (gengotypes FLATTENS an embedded def into the parent
// struct; verified against cue v0.16.1). It deliberately does NOT carry `approval`,
// which both step defs declare with their own type.
#PipelineFlow: {
	id!:         string & !=""
	when?:       string & !=""
	env?:        {PATH?: _|_, [string]: #StrVal} @go(Env,type=map[string]string)
	cwd?:        string & !=""
	stdin?:      string & !=""
	timeout_ms?: int & >0 @go(TimeoutMs)
	on_error?:   "fail" | "continue" | "skip_rest" @go(OnError)
	retry?:      #PipelineRetry
}

// #PipelineArms — the EXEC arms the authored pipeline step carries. Declared ONCE here
// (R3) and embedded by #PipelineStepBase, so the arms cannot drift from the step that
// uses them; #PipelineSubStep deliberately re-declares its own NON-RECURSIVE subset
// (lobster's rule: no nested parallel/for_each/input/workflow inside a branch).
//
// It deliberately does NOT carry `approval`: that lives on #PipelineStepBase, because a
// step INSIDE `parallel.branches[]` / `for_each.steps[]` must not open a gate. The
// exec-arm XOR (exactly one of them) is a Go rule in plugin-pipeline's OpValidate, not a
// CUE disjunction.
#PipelineArms: {
	// --- lobster exec arms ---
	run?:           string & !=""
	pipeline?:      string & !=""
	workflow?:      string & !=""
	workflow_args?: {[string]: #StrVal} @go(WorkflowArgs,type=map[string]string)
	parallel?:      #PipelineParallel
	for_each?:      string & !="" @go(ForEach)
	input?:         #PipelineInput

	// --- for_each companions (meaningful only alongside for_each) ---
	item_var?:   string & !="" @go(ItemVar)
	index_var?:  string & !="" @go(IndexVar)
	batch_size?: int & >0 @go(BatchSize)
	pause_ms?:   int & >=0 @go(PauseMs)
	steps?:      [...#PipelineSubStep]

	// --- charly arms: the FULL charly grammar, any plugin of any class ---
	plan?:   [...#Step]
	charly?: [string, ...string] | (string & !="") @go(Charly,type=[]string)
}

// #PipelineInput — the lobster `input` gate. `response_schema` is a JSON Schema
// document; the engine validates the resume response against it
// (cuelang.org/go/encoding/jsonschema). The `.lobster` importer maps lobster's own
// `responseSchema` spelling onto this charly snake_case key.
#PipelineInput: {
	prompt!:          string & !=""
	response_schema?: {[string]: _} @go(ResponseSchema,type=map[string]any)
	defaults?:        {[string]: #StrVal} @go(Defaults,type=map[string]string)
}

// #PipelineSubStep — a step INSIDE `parallel.branches[]` or `for_each.steps[]`. It is
// NON-RECURSIVE by construction (lobster's own rule): no nested parallel/for_each/
// input/workflow and no approval, so a workflow stays flat and exportable.
#PipelineSubStep: {
	// The SAME flow block every other step def carries — embedded, never
	// re-declared (R3). gengotypes flattens it exactly as it does for
	// #PipelineStepBase, so the substep's Go shape is unchanged.
	#PipelineFlow

	run?:      string & !=""
	pipeline?: string & !=""
	plan?:     [...#Step]
	charly?:   [string, ...string] | (string & !="") @go(Charly,type=[]string)
}

// #PipelineParallel — lobster `parallel`. `wait: "any"` returns the first branch and
// cancels the rest; `wait: "all"` (the default) collects every branch.
#PipelineParallel: {
	wait?:       *"all" | "any"
	timeout_ms?: int & >0 @go(TimeoutMs)
	branches!:   [#PipelineSubStep, ...#PipelineSubStep]
}

// #PipelineStepBase — one authored workflow step.
#PipelineStepBase: {
	#PipelineFlow
	#PipelineArms

	// approval — terse authored union; an engine normalizes it to #PipelineApproval.
	approval?: bool | string | #PipelineApproval @go(Approval,type=any)
}

// #PipelineStep — the step type `steps:` carries: the authored step, named so a step
// consumer (sdk/workflowkit) has a stable type to build on without reaching for the
// base def.
#PipelineStep: #PipelineStepBase

// #PipelineSchedule — the cron trigger. A 5-field cron, the SAME grammar
// deploy.schedule, a k8s CronJob and GitHub Actions `on.schedule` use, so one
// authored schedule lowers to every consumer; the lobster engine converts it to a
// systemd `OnCalendar` (validated with `systemd-analyze calendar`).
#PipelineSchedule: {
	cron!:       string & !=""
	timezone?:   string & !=""
	args?:       {[string]: #StrVal} @go(Args,type=map[string]string)
	persistent?: bool
}

// #PipelineTrigger — how a pipeline starts. Both fields optional; a trigger setting
// NEITHER is a load error raised in Go (a closed CUE struct cannot express
// "exactly one arm" without a disjunction — see the file header).
#PipelineTrigger: {
	manual?:   bool
	schedule?: #PipelineSchedule
}

// #Pipeline — the authored `kind: pipeline` entity body.
//
// `entities:` holds inline entities of ANY kind plugin (a task, a candy, a deploy,
// another pipeline, …). It is typed as an opaque map because #Node is EXCLUDED from
// param-gen (the plugin SDK contract forbids base refs in a self-contained schema);
// each inline entity is validated by its OWN kind's value def and OpValidate once
// the generated charly.yml is loaded.
#Pipeline: {
	description!: string & !=""
	// engine — the workflow engine word, resolved against the `workflow` PROVIDER
	// CLASS and dispatched as InvokeProvider("workflow", <engine>, workflow-run|…)
	// through the normal path. Default "lobster".
	engine?:     string & !=""
	args?:       {[string]: #TaskParamSpec}
	env?:        {PATH?: _|_, [string]: #StrVal} @go(Env,type=map[string]string)
	cwd?:        string & !=""
	cost_limit?: number & >=0 @go(CostLimit)
	triggers?:   [...#PipelineTrigger]
	// config — the former pipeline knobs (llm/media/report/skills/channels/gates/
	// repo/redo/agent), carried opaquely so a knob the engine does not know is not a
	// schema change.
	config?:   {[string]: _} @go(Config,type=map[string]any)
	entities?: {[string]: {...}} @go(Entities,type=map[string]map[string]any)
	steps!:    [#PipelineStep, ...#PipelineStep]
}

// #PipelineValue — the HOST-SIDE value gate def for the `pipeline` kind (the
// #TaskValue precedent). @go(-): the Go type comes from #Pipeline, so this def is
// validation-only — schemagen derives KindValueDefs["pipeline"] from its NAME.
#PipelineValue: #Pipeline @go(-)

// #PipelineClassRoutes — the class→route table: how EVERY class in
// #ProviderClassNames is reached from inside a workflow. schemagen emits
// spec.PipelineClassRoutes from it (the #ProviderClassNames precedent — a @go(-) def
// extracted into a Go table, never a hand-maintained copy), so spec's own
// TestPipelineClassCoverage (spec/pipeline_test.go) can fail the build the moment a
// provider class is added without a decided workflow route — and the moment a row
// names a class that is not in the vocabulary (R2: a new class must FORCE the
// decision, never silently get no route; and a stale row must not outlive its class).
//
// Each value names the step arm that reaches the class. The lowering special-cases no
// word: it only emits plan steps, argv and entity nodes, so a plugin released tomorrow
// is usable in a workflow with no change here.
#PipelineClassRoutes: {
	"kind":          "entities: — an inline node, or a project entity referenced by include:/task:/workflow:/charly:"
	"deploy":        "entities: + charly: [deploy, add|del|show|status, <name>] / charly: [start|stop|remove, <box>] / charly: [check, run, <bed>]"
	"verb":          "plan: — a `<word>: <input>` step (single-step sugar is plan: with one step)"
	"step":          "plan: — the run:/check:/agent-run:/agent-check:/include: intents, with every #Op modifier"
	"build":         "entities: candy/box + charly: [box, build|generate, <name>]"
	"builder":       "inside those candy plans — a plan build:/<verb>: step reaches the builder legs host-side"
	"command":       "charly: [argv…] — any command-class plugin, incl. nested command:<word>:<parent>"
	"engine":        "the entity's engine: field (pod/box), exactly as today"
	"workflow":      "the engine plugin: from INSIDE a workflow, a `workflow:` sub-workflow step reaches another engine; the front-end reaches this class host-side via InvokeProvider(\"workflow\", <engine>, …)"
	"loader":        "implicit — the generated charly.yml is loaded by the plugin loader on every charly step"
	"refs":          "implicit — import:/@github… refs in the generated charly.yml resolve through it"
	"agent-runtime": "plan: — the agent-run:/agent-check: intents"
	"terminal":      "charly: [shell|tui, …] reaches the terminal providers"
} @go(-)
