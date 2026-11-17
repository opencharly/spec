package schema

// CUE schema for the PIPELINE STAGE vocabulary — DESIGN.md D-FLOW-1/2/3, transcribed from
// DESIGN.cue:894-915 (the contract's own written shape). P26's unit 1.
//
// WHY THIS LIVES IN `spec` AND NOT IN plugin-pipeline: the stage shape is a CONTRACT shape. The
// plugin's own schema header says its module may import only the contract module, so these defs land
// here FIRST and the plugin's `#Pipeline.steps` then becomes `stage: [...#Stage]` built from them.
// The ledger's P26 row had this order INVERTED (`plugin-pipeline > spec`) until the 2026-11-16 audit
// measured it.
//
// WHAT IS DELIBERATELY NOT HERE: a `#DeploymentStep` vocabulary. DESIGN.cue writes
// `step: {step!: [...#DeploymentStep]}` because it is a standalone document; in THIS module that
// vocabulary already exists as `#Step` (00_common.cue:46) / `#Op` (:59), so the step arm REUSES it
// rather than restating forty defs (R2: one definition of the plan vocabulary).
//
// THE FACT THAT MAKES THE ROSTER OBSOLETE (DESIGN.md:2996, charly#1168): a stage runs steps, runs
// another pipeline, realizes deployments and runs their checks, or asks the operator — and `after:`
// orders stages. A flat `check-roster` kind with its own `lanes` and worker pool is a SECOND RUNNER,
// which DESIGN.md refuses; a roster becomes
// `stage: {<bed>: {deployment: [...], after: [...producers], concurrency: N}}`.

// #Count / #Text — DESIGN.cue:53-54's two scalar primitives, transcribed because this module had no
// def for either and a stage's `attempt`, `concurrency` and `prompt` are exactly them. ONE definition
// each (R2).
#Count: int & >0
#Text:  string & !=""

// #Retry — D-FLOW-2: a stage's repetition is a DECLARED, BOUNDED count. It never repeats a failed
// plugin call (D-PROTO-7) and is never blind (D-LAW-5), which is why `attempt` is bounded by #Count
// and `until` names the condition that ends it. This REPLACES plugin-pipeline's timed
// `#PipelineRetry{max, delay_ms, factor}`; row 2.22's `attempt` is therefore `retry.attempt`
// (DESIGN.cue:563-567), not a top-level field.
#Retry: {
	until?:   #Duration
	attempt?: #Count
	interval: *"2s" | #Duration
}

// #Lane — D-FLOW-1: a stage may run once per item of the list a parameter holds.
#Lane: over!: #EntityRef

// #StageName / #StageRef — what a stage's `after:`/`when:` name and what its arms hold. Both use this
// module's ONE name grammar (#EntityRef) rather than a second one.
#StageName: #EntityRef
#StageRef:  #EntityRef

// #StageWhen — the parameter matcher map a stage's `when:` holds.
#StageWhen: {[string]: #Matcher}

// #StageAlt — D-FLOW-1: a stage does EXACTLY ONE thing. Four arms and never a fifth field.
//
// THE SCHEDULING FIELDS ARE DECLARED HERE, ONCE, BESIDE THE ARMS — not unified into each arm.
// DESIGN.cue writes each arm as `{step!: [...], #StageFields...}`; in this module BOTH the postfix
// spread and the `&` unification are unusable, measured: the spread needs an experiment flag, and
// `{name!: T} & #StageFields` fails composition with `#<Arm>.<field>: field not allowed` for every
// required field name tried (and `#StageAsk & #StageFields` fails the same way). Hoisting the shared
// fields onto the sum is the shape that compiles, generates typed Go, and still reads as
// "a stage has scheduling fields and one arm".
//
// TWO FURTHER MEASURED DEVIATIONS, both deliberate (§7g's class — a silent difference would be the
// defect, a recorded one is not):
//   - the ask arm carries `#StageAsk` DIRECTLY rather than nesting a second `ask:` inside the `ask:`
//     arm as DESIGN.cue spells it; a REQUIRED field named `ask` cannot be unified with anything here,
//     and the direct form is one nesting shallower with the same fields.
//   - `#Stage: or([for _, a in #StageAlt {a}])` cannot be written; `or()` over the arms fails the same
//     way. The sum is four optional arms, and "exactly one arm" is a VALIDATION rule, not a schema one.
#StageAlt: {
	after?:      [...#StageName] // the stages this one runs AFTER — the ordering edge (D-FLOW-1)
	when?:       #StageWhen      // the stage runs when every named parameter matches
	timeout?:    #Duration
	retry?:      #Retry
	on_error:    *"fail" | "continue"
	lane?:       #Lane
	concurrency: *1 | #Count // how many lanes or deployments run at once

	step?:       #StageStep       // a stage that observes the host venue
	pipeline?:   #StagePipeline   // a stage that runs another pipeline
	deployment?: #StageDeployment // a stage that realizes deployments and runs their checks
	ask?:        #StageAsk        // a stage that asks the operator (D-FLOW-3)
}
#Stage: #StageAlt

// The four arms, each a PURE payload: the scheduling fields live on #StageAlt above.
#StageStep:       {step!: [...#Step]}
#StagePipeline:   {pipeline!: #StageRef}
#StageDeployment: {deployment!: [...#StageRef]}

// #StageAsk / #Answer — D-FLOW-3: an ask carries its ANSWER CHANNEL, so a question with no channel is
// not an ask. The response must satisfy `expect:` to be `granted`; silence takes `unanswered:`,
// refused by default.
#StageAsk: {
	prompt!: #Text
	answer!: #Answer
}
// #Answer.unanswered (D-FLOW-3) — three words, so a VOCABULARY TABLE (S3), not an inline
// union: D-PAT-3 allows an inline literal default only for a two-value set, and a table makes
// the enum and the words unforgeable from each other (a new word is ONE row here).
_answerUnanswered: {
	refused:   {}
	granted:   {}
	cancelled: {}
} @vocab(spec)

_answerUnansweredWord: [for w, _ in _answerUnanswered {w}]

#Answer: {
	expect?:    #Matcher
	unanswered: *"refused" | or(_answerUnansweredWord)
}
