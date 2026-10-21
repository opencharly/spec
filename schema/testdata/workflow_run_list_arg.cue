// The RUNTIME-LIST-ARG fixture for the workflow engine wire.
//
// WHY A FILE AND NOT ONLY AN INLINE LITERAL. This is the document the chain's lane
// primitive needs to be able to send: one named arg whose value is a LIST of PR
// numbers, alongside a scalar arg. It is kept as a concrete CUE document so the shape
// can be validated by the schema's own toolchain (`cue vet`) and not only by a Go test
// that could drift from the CUE.
//
// It lives under `schema/testdata/` on purpose: `schema/schema.go` embeds `*.cue` at the
// FS ROOT only and `schemaconcat.ConcatSchema` skips directories, so this file is never
// spliced into the shipped schema. It is concatenated with the schema only by the
// caller that wants to validate it — `spec/workflow_arg_list_wire_test.go` and the
// `cue vet` invocation in the PR body. One fixture, two consumers, no copies.
//
// The fields are PUBLIC (no leading `_`) because `cue.Value.LookupPath` cannot address a
// hidden label: a `_name` fixture validated under `cue vet` while the Go consumer could
// not see it at all. A fixture that both consumers must address has to be addressable
// by both.
//
// No package clause, exactly like every `schema/*.cue` file: the concatenation supplies
// the package, and a clause here would start a second one.

// The input document: a run request carrying a list arg. `prs` is the retired lane
// model's `--prs a b c`; `who` is the scalar control that must keep working unchanged.
workflowRunListArg: {
	pipeline: "lane"
	args: {
		prs:    ["101", "102", "103"]
		who:    "world"
		labels: ["a", "b"]
	}
}

// The ASSERTION: the document is unified with the closed request def, so a schema that
// stopped admitting a list fails here with a concrete CUE error instead of silently
// accepting the document.
workflowRunListArgRequest: #WorkflowRunRequest & workflowRunListArg

// The same value must survive the schedule path, which normalizes an authored
// `#PipelineSchedule.args` into `#WorkflowSchedule.args` verbatim. Both sides are
// asserted so the two defs cannot drift into "the run accepts a list but the schedule
// does not".
workflowScheduleListArg: #PipelineSchedule & {
	cron: "* * * * *"
	args: workflowRunListArg.args
}
workflowTriggerListArg: #WorkflowSchedule & {
	cron: "* * * * *"
	args: workflowRunListArg.args
}
