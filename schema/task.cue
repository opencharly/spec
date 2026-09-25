// CUE schema for the generic `task` kind. A task is a named, host-native,
// REUSABLE plan — the SAME #Step grammar a candy's `plan:` uses, plus
// task-level execution config (workdir, env, vars, incremental staleness,
// dependencies, parameters). It is the declarative replacement for a repo
// task-runner entry: any task a repository needs — build, test, lint, release,
// verify, notify — is authored as a `task:` node in charly.yml and run with
// `charly task <name>`.
//
// #Task references the BASE #Step/#Op grammar (unlike a self-contained plugin
// schema), so it MUST live in the base schema and is validated HOST-SIDE
// against #TaskValue — the same pattern #Candy/#Local/#Vm already use. A
// self-contained plugin schema cannot carry `plan: [...#Step]` (the plugin SDK
// contract forbids base refs); the host value gate is the correct home. See
// /charly-internals:plugin "Why self-contained schemas".
//
// The word→def map the host gate consults (spec.KindValueDefs) is DERIVED from
// this `#TaskValue` def by schemagen — adding this kind needs no hand-maintained
// map entry (see the kindValueDefs comment in internal/schemagen/main.go).

// #TaskParamSpec — the typed declaration of one task parameter: its prose, an
// optional default, and whether it is required. Values are passed on the CLI as
// --param NAME=VALUE and substituted into the plan's ${NAME} references.
#TaskParamSpec: {
	description?: string & !=""
	default?:     #StrVal
	required?:    bool
}

// #Task — one generic task entity.
//
// The first three fields mirror the Go Task runner's authoring surface
// (dir/env/vars/depends_on), the next block its incremental/staleness model
// (sources/generates/status/preconditions), then the execution knobs
// (silent/interactive/platforms/timeout/continue_on_error), then typed params,
// and finally the ORDERED `plan:` — the reused #Step grammar whose steps run in
// authored order on the host (or a venue the step selects).
//
// `description!` is required (the ADE identity contract every entity carries).
// A `plan:` is optional in the schema; the plugin's own OpValidate requires at
// least one step so `charly task <name>` has something to run.
#Task: {
	// --- identity (required: ADE) ---
	description!: string & !=""

	// --- execution context ---
	// dir — the working directory every step runs in. Relative paths resolve
	// against the task's project root (the directory holding charly.yml);
	// ${VAR} references resolve against env/vars at run time.
	dir?: string & !=""
	// env — extra environment variables exported for the task's steps. PATH is
	// reserved (use the shell profile); values are Go-coerced scalars.
	env?: {PATH?: _|_, [string]: #StrVal} @go(Env,type=map[string]string)
	// vars — task-local ${VAR} substitution values (like a candy's var:), a
	// build/run-time map of string values.
	vars?: {[=~"^[A-Z_][A-Z0-9_]*$"]: #StrVal} @go(Vars,type=map[string]string)

	// --- dependencies ---
	// depends_on — task names that must complete (in order) before this task
	// runs. `charly task <name>` runs the closure; a cycle is a load error.
	depends_on?: [...#EntityRef] @go(DependsOn)

	// --- incremental / staleness model (Go-Task parity) ---
	// sources — glob paths whose modification invalidates the task.
	sources?: [...string]
	// generates — glob paths the task produces; if all are newer than every
	// source, the task is considered up to date and skipped (unless --force).
	generates?: [...string]
	// status — up-to-date probe commands; each is a shell command whose exit 0
	// marks the task up to date (all must pass for a skip).
	status?: [...string]
	// preconditions — shell commands that must exit 0 before the task runs; a
	// failing precondition aborts the task (unlike status, which skips).
	preconditions?: [...string]

	// --- execution knobs ---
	silent?:           bool
	interactive?:      bool
	platforms?:        [...string]
	exclude_platforms?: [...string] @go(ExcludePlatforms)
	timeout?:          #Duration
	continue_on_error?: bool @go(ContinueOnError)

	// --- typed parameters ---
	params?: {[string]: #TaskParamSpec}

	// --- the plan (the reused grammar) ---
	plan?: [...#Step]
}

// #TaskValue — the HOST-SIDE value gate def for the `task` kind (the #Candy /
// #<Kind>Value analogue). @go(-): the Go type comes from #Task directly, so
// this def is validation-only (schemagen derives KindValueDefs["task"] from it).
#TaskValue: #Task @go(-)
