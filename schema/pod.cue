package schema

// CUE schema for the `pod` KIND's shared sidecar wrapper.
//
// THE KIND BODY IS NOT HERE. `#Pod` and its host-side value-gate def `#PodValue` live with
// the plugin that serves the kind — `plugin-substrate`'s own CUE module,
// `candy/plugin-substrate/schema/pod.cue`. opencharly/plugin-substrate#32 (spec#304 unit 3's
// leg 1) moved them there, and the plugin's `kind` capability DECLARES
// `InputDef: "#PodValue"`, so the host resolves the gate def from the provider's declaration
// rather than from this module. Unit 3's leg 2 (this module) deleted the twins, so `pod` is
// no longer in `spec.KindValueDefs` and the generated `spec.Pod` type is gone.
//
// WHAT STAYS, AND WHY. `#PodSidecar` is this kind's KEEPER: the CONTRACT's own
// `#ResolvedPod` (`schema/substrate_template.cue`) cites it, and that envelope is what the
// kernel reads (the include-spliced plan). It is not the same reason `#TaskParamSpec` stays —
// that one is shared by a SECOND kind. This one stays because a def a kernel-facing wire
// shape references cannot move into one plugin's CUE module.

// #PodSidecar mirrors Go SidecarConfig: a single-key wrapper around a map of
// named sidecar templates. CLOSED (the struct has exactly one field).
#PodSidecar: {
	sidecar: {[string]: #Sidecar}
}
