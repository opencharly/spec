// The OUT-OF-TREE plugin-tunnel's OWN CUE schema — the typed params for the
// `tunnel` VERB (verb:tunnel), the externalized tailscale/cloudflare TUNNEL
// EXECUTION LEG. It is the SINGLE SOURCE for this plugin's params, used two ways
// (the same contract core `spec` + the reference examplerunverb use):
//
//  1. GENERATE the Go param struct — the schema→Go pipeline concatenates this file under a
//     `package params` + `@go(params)` header and generates the Go struct from it,
//     emits
//     ../params/cue_types_gen.go, so the provider decodes plugin_input into a TYPED
//     struct (params.TunnelInput / TunnelConfig / TunnelPort), never a hand-parsed map.
//  2. VALIDATE authored input AT RUNTIME — the plugin SERVES this source over the
//     Describe channel (sdk.BuildCapabilities → schema_cue); the host splices it onto
//     its base (base ++ plugin) and validates every authored `plugin: tunnel` step's
//     plugin_input against #TunnelInput. The host NEVER reads this file from disk — the
//     schema travels with the plugin.
//
// verb:tunnel is NOT a check verb in the usual sense — it is the externalized TUNNEL
// EXECUTION backend. The pod-lifecycle plugins (candy/plugin-deploy-pod, candy/plugin-pod)
// drive its start/stop/setup methods directly over this verb's Invoke envelope
// ({method,config}) via InvokeProvider. It ALSO
// carries a benign `plan` (dry-run) method — a `plugin: tunnel` check step returns the
// EXACT tailscale/cloudflared argv it WOULD run WITHOUT exec, so a disposable bed proves
// the registry dispatch + the TunnelConfig wire round-trip + the moved command-building
// logic with ZERO tailscale/cloudflare credentials.
//
// SELF-CONTAINED: #TunnelInput references only its sibling defs (#TunnelConfig /
// #TunnelPort) WITHIN this schema, NO base def, so it compiles standalone (gengotypes +
// the SDK serve-side check) AND splices onto the base — the splice exists to detect a
// def-name collision with the base, not to resolve base references.

import "github.com/opencharly/spec/schema"

// #TunnelInput — the plugin_input shape for the `tunnel` verb.
#TunnelInput: {
	// method — the tunnel operation: start | stop | setup | plan. `plan` is the
	// creds-free dry-run that returns the argv the operation WOULD run (no exec).
	method: string & !="" @go(Method)
	// config — the resolved tunnel configuration to act on (byte-compatible with the
	// core's TunnelConfig, sent by the calling pod-lifecycle plugin over the Invoke envelope).
	config?: schema.#TunnelConfig @go(Config)
	// expect — plan only: the expected argv command lines (space-joined). When set, the
	// plan method compares the built argv against it (FAIL on mismatch); empty ⇒ the
	// step passes as long as dispatch + round-trip succeed, echoing the built argv.
	expect?: [...string] @go(Expect)
}
