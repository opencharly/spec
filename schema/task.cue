package schema

// The `task` kind's SHARED parameter contract.
//
// The rest of the kind's schema — the `#Task` authored entity body and its `#TaskValue`
// host-side value-gate def — lives with the plugin that serves the kind
// (`opencharly/plugin-task`'s own CUE module, `candy/plugin-task/schema/task.cue`) since
// opencharly/spec#304 unit 2. A `kind` body belongs with the plugin that serves it, and the
// host resolves the gate def from the provider's DECLARED `InputDef` (`#TaskValue`), so this
// module no longer declares `#Task`/`#TaskValue` and `task` is no longer in
// `spec.KindValueDefs`.
//
// THIS def STAYS, because it is SHARED and no plugin can own it: a `kind: task`'s
// `params:` and a `kind: pipeline`'s `args:` are the same contract —
// `plugin-pipeline`'s schema types its `args:` as `{[string]: schema.#TaskParamSpec}` — and
// a plugin's CUE module can import only THIS contract module, never another plugin's.
// Moving it would let the two kinds disagree about what an arg value is (R2).
//
// #TaskParamSpec — the typed declaration of one task parameter: its prose, an
// optional default, and whether it is required. Values are passed on the CLI as
// --param NAME=VALUE and substituted into the plan's ${NAME} references.
#TaskParamSpec: {
	description?: string & !=""
	default?:     #StrVal
	required?:    bool
}
