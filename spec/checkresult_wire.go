package spec

// checkresult_wire.go — the check-run REPLY wire types (#55 CHECK-ENGINE cone Option A).
// The per-step result + the check-run reply envelope, homed in the spec contract module so
// charly core's check-run seam (host_build_check_run.go) + the deploy-verify path
// (check_cmd.go) reference them importing only spec, and sdk/kit re-exports each (sdk/kit/
// check_result.go) so every candy call site compiles UNCHANGED. These are HAND-WRITTEN wire
// types (the same wire-mandate exception as CheckRunReply/StepPass always carried — see the
// sdk/kit/checkrun_seam.go header: gengotypes cannot express kit.CheckResult's engine-internal
// `DeadlineExceeded bool json:"-"`, so the wire form carries the spec.CheckResult fields only
// and the DeadlineExceeded flag lives on the kit-internal engine CheckResult, dropped at the
// StepResult boundary exactly as it was on the wire before — byte-identical output, R3).
//
// The wire shape is byte-identical to the former kit.StepResult/kit.CheckRunReply/kit.StepPass:
// kit.StepResult.Result was kit.CheckResult (spec.CheckResult embedded + DeadlineExceeded
// json:"-"), so on the wire only the spec.CheckResult fields ever crossed; spec.StepResult.Result
// is spec.CheckResult directly — the SAME wire bytes. charly unmarshals the plugin-check reply
// (kit.StepResult JSON) into spec.StepResult transparently.

// StepResult is one plan step's outcome — the step's identity (keyword/text/origin/id) plus
// the CheckResult of running it. The result reporters consume a []StepResult.
type StepResult struct {
	Keyword string      `json:"keyword"`
	Text    string      `json:"text"`
	Origin  string      `json:"origin,omitempty"`
	StepID  string      `json:"step_id"`
	Result  CheckResult `json:"result"`
}

// CheckRunReply is the host-resolved result of a check-run. Steps is the per-step verdict
// list the plugin formats (FormatStepResults*) and tallies into an exit code. Image is the
// resolved image ref for the "Image: <ref>" header line. NoSteps signals the image declared no
// plan (the plugin prints "No plan steps defined for this image." and exits 0) — distinct from
// an empty Steps that ran zero scored steps. The host signals an infra error (bad image, engine
// failure) via the builder's error return, surfaced to the plugin.
type CheckRunReply struct {
	Steps   []StepResult `json:"steps,omitempty"`
	Image   string       `json:"image,omitempty"`
	NoSteps bool         `json:"no_steps,omitempty"`
	// Header is the pre-formatted, kind-specific banner line the host builds ("Image: X
	// (container: Y)" for pod, "VM: <name> (ssh …)", "Local deploy: …", "Group bed: …") from
	// data only the host holds (container name, ssh user/host/port, member count), so the
	// plugin stays kind-blind: it prints Header, then the formatted Steps.
	Header string `json:"header,omitempty"`
	// Passthrough carries the one non-plan-run live path — a nested pod-in-VM leaf whose check
	// the host delegates to the guest over SSH (`charly check live <pod>` run INSIDE the guest),
	// whose stdout/stderr + exit code the plugin forwards verbatim. Nil for every plan-run mode.
	Passthrough *StepPass `json:"passthrough,omitempty"`
	// Score is the "score"-mode reply (originally P12 Wave-2; the mode dispatches directly
	// plugin-side since K1-unblock wave arm 3): the AI-harness SCORING result — the plugin's own
	// pluginRunCheckLive's per-step verdicts (the substituted, nonce-carrying scoring plan walked
	// plugin-side) the plugin scorer consumes (summary, StepByID, Classify). Nil for the
	// box/live/feature plan-run modes, which carry their verdicts in Steps. CUE-sourced
	// (spec.CheckRunResults) so this same definition still serves the wire shape uniformly
	// (SDD; no alias) even though both producer and consumer are now plugin-side.
	Score *CheckRunResults `json:"score,omitempty"`
}

// StepPass is the verbatim stdout/stderr/exit-code of a host-delegated guest sub-invocation
// (the nested pod-in-VM check-live delegation, runVm's guestNestedCheckCmd path). The plugin
// writes Stdout/Stderr and returns ExitCode unchanged, so a guest-run check reports
// byte-identically to a direct one. Hand-written (not CUE): it is part of the kit reply model,
// which the wire mandate's spike keeps hand-written alongside CheckRunReply.
type StepPass struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}