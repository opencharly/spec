// Run-scoped observation instruments + the machine-written evidence envelope
// (Cutover A of the nested-capture architecture). `instrument:` entries sit on
// any deployable substrate-node body (deploy.cue, beside disposable:/lifecycle:)
// and carry a capture verb from the catalog (spice/record/wl/vnc/libvirt/cdp/
// adb/appium — ANY plugin word), a run-phase bracket, and an optional post-run
// pipeline. The bed runner owns instruments across run phases; evidence rows
// land in ONE manifest (evidence.yml) and pipelines are blind word dispatches —
// core and the runner never branch on capture kind. Shared defs REFERENCED, not
// redefined (R3): #Context/#Duration/#CalVer live in _common.cue.

// #Phase — the RUN-PHASE bracket enum: which check-run phase brackets an
// instrument's capture segment (build = the image/domain build steps; live =
// the check-live pass; update = the update/rebuild/re-verify step; teardown =
// the cleanup steps). DISTINCT from #Context ("build"|"deploy"|"runtime",
// _common.cue — a step-EXECUTION-legality enum) and from the build-vocabulary
// #PhaseSet/#PhaseTemplates (the prepare/install/cleanup template set): this
// is the RUN lifecycle, never a build phase nor an exec context. @go(-): NO
// Go alias is emitted for the def name — `Phase` is already TAKEN in the spec
// package by the build-vocabulary IR enum (spec/ir_enums.go,
// PhasePrepare/PhaseInstall/PhaseCleanup), so the two referencing fields carry
// explicit @go(type=) pins (`[]string` / `string`) instead — the scalar-collapse
// contract of scalar_aliases.go (hand-written, never generated).
#Phase: ("build" | "live" | "update" | "teardown") @go(-)

// #PipelineWord — ONE post-run pipeline stage: a single plugin-VERB dispatch,
// authored in its sugar form `<word>: <input>` (the plan's
// `pipeline: [{transcode: {to: mp4}}]` shape — a map whose single key is the
// verb and whose value is that verb's own input) and desugared at parse time
// into the internal plugin/plugin_input pair below — the EXACT desugar
// contract plan steps use (see #Op's pair in _common.cue). CLOSED: the
// desugar consumed the OPEN verb word, so the desugared entry validates
// against just the pair; a two-verb-key entry is at-most-one-verb-violating
// and cannot reach this def (the parse-time desugar hard-errors it — the same
// exactly-one-verb discipline #Step/#Op enforce). plugin_input is validated by
// the PLUGIN's own served CUE schema (not this def).
#PipelineWord: {
	plugin?:       string
	plugin_input?: {...} @go(PluginInput,type=map[string]any)
}

// #Instrument — one run-scoped observation entry on a substrate-node body
// (`instrument:` beside `disposable:`). CLOSED: a misspelled field is a typo.
// The capture verb is the generic plugin sugar `<word>: <input>` — the same
// discriminator pattern #Step uses: the parse-time desugar rewrites it into
// the internal plugin/plugin_input pair BEFORE this def validates, so a plugin
// word never appears here (authoring plugin:/plugin_input: directly is a hard
// load error, run: charly migrate) and exactly-one-verb-per-instrument is the
// parse-time + closedness discipline (a second verb sugar key is an unknown
// field). The venue an instrument captures is DERIVED from its node's
// fleet-tree position (like a step's venue) — never authored here.
#Instrument: {
	// id — the authored observation identity; the bed runner scopes it per
	// venue (`<bed>.<member>.<id>`).
	id?: string @go(ID)
	// phase — the run-phase brackets this instrument captures in. Default
	// ["live"] (the common single-run observation); a [live, update] span
	// yields TWO evidence segments across the R10 rebuild (the venue is
	// recreated — honest, never re-attached).
	phase?: *["live"] | [...#Phase] @go(Phase,type=[]string)
	// pipeline — post-run word dispatches over the instrument's artifacts
	// (e.g. `- transcode: {to: mp4}` → plugin-media), executed in the
	// evidence phase BEFORE venue teardown. Blind dispatches: the runner owns
	// zero format knowledge; a new pipeline verb joins by serving its own
	// schema, with no core change.
	pipeline?: [...#PipelineWord]
	// --- the desugared verb pair — INTERNAL-ONLY, mirroring #Op's exact
	// declarations (the authored `<word>: <input>` sugar rewrites here) ---
	plugin?:       string
	plugin_input?: {...} @go(PluginInput,type=map[string]any)
	// --- the shared modifiers an instrument meaningfully shares with #Op ---
	timeout?: #Duration
	context?: [...#Context]
}

// #EvidenceRow — ONE machine-written evidence-manifest row
// (.check/<bed>/<calver>/evidence.yml), emitted by the bed runner + the
// session providers through the shared evidence path. CLOSED: a misspelled
// field on an emitted row is a typo. Segment elements carry the open `...`
// tail (machine-written spans evolve forward; the #VmDeployState precedent).
#EvidenceRow: {
	// instrument — the venue-scoped capture identity this row belongs to
	// (`<bed>.<member>.<id>`).
	instrument!: string & !=""
	// origin — where the capture came from: a background SESSION the runner
	// owned, a plan STEP that recorded inline, or a structured SUB-RUN
	// (aggregate include).
	origin!: "session" | "step" | "sub-run"
	// verb — the capture verb word that produced the artifacts ("spice",
	// "record", "wl", "vnc", "transcode", ...). OPEN — it names a dispatched
	// word, never a closed enum.
	verb!: string & !=""
	// venue — the fleet-tree venue identity the capture ran on (the derived
	// venue word, matching the step venue vocabulary #Op.venue).
	venue!: string & !=""
	// phase — the run-phase bracket the capture segment sat in.
	phase?: #Phase @go(Phase,type=string)
	// segment — the capture span(s): ONE per run-phase bracket an instrument
	// spanned (a [live, update] instrument yields two).
	segment?: [...#EvidenceSegment]
	// artifact — the produced files: absolute/run-relative path + an OPEN kind
	// word ("mjpeg"|"mp4"|"cast"|"gif"|... — never a closed enum: new capture
	// kinds join the model freely) + optional validator expectations.
	artifact?: [...#EvidenceArtifact]
	// pipeline — the post-run word dispatches executed over these artifacts
	// (the executed form of the instrument's pipeline; same desugared shape).
	pipeline?: [...#PipelineWord]
}

// #EvidenceSegment — ONE capture span (start → stop) with optional frame/byte
// counts. start/stop are flexible strings: a canonical CalVer (#CanonCalVer)
// OR any wall-clock timestamp — the capture span is run time, not a schema
// stamp, so the type is deliberately never pattern-constrained. Machine-
// written; the open `...` tail is the forward-evolution hatch for a state
// record (the #VmDeployState precedent) — which degrades the whole def for
// gengotypes, so the generated `EvidenceRow.Segment []EvidenceSegment` IS
// `[]map[string]any`: the flexible shape the machine-written rows need, with
// CUE still validating start/stop/frames/bytes when present.
#EvidenceSegment: {
	start:  string
	stop:   string
	frames?: int
	bytes?:  int
	...
}

// #EvidenceArtifact — one produced capture artifact. CLOSED: the path + the
// OPEN kind word + the optional validator map (validator semantics — RDD-3
// binding: artifact_not_uniform on video evaluates SOURCE frames pre-encode or
// a pixel-delta threshold on decoded output; exact hashes of transcoded frames
// are vacuous and MUST NOT be authored).
#EvidenceArtifact: {
	path!:       string & !=""
	kind!:       string & !=""
	validators?: {...}
}
