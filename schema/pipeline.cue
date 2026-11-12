package schema

// CUE schema for the `pipeline` KIND's class→route table.
//
// THE KIND BODY IS NOT HERE. `#Pipeline` / `#PipelineValue` and the step / trigger /
// flow defs they are built from belong with the plugin that serves the kind, and they
// live in `plugin-pipeline`'s own CUE module
// (`github.com/opencharly/plugin-pipeline/candy/plugin-pipeline/schema/pipeline.cue`).
// Unit 1 of opencharly/spec#304 moved them there (leg 2), declared the body def on the
// plugin's `kind` capability (`InputDef: "#PipelineValue"`), and this module's last leg
// deleted the twins: a `kind` body has ONE home, and it is the plugin's.
//
// WHAT STAYS, AND WHY. `#PipelineClassRoutes` is a table over THIS module's own
// `#ProviderClassNames` — every class a plugin can be invoked through — so it cannot
// live in a plugin: no plugin sees the whole class vocabulary, and no plugin can run
// spec's vocabulary generator (`internal/schemagen`), which is what extracts this def
// into `spec.PipelineClassRoutes` and fails the build the moment a class is added
// without a decided workflow route. It is a contract fact about the VOCABULARY, not
// about the pipeline body, and it outlives the body's move.

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
