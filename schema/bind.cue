package schema

// bind.cue — the binding contract (§8), PORTED from DESIGN.cue, not re-designed.
//
// DESIGN.cue declares `bind`/`unbind` on the `type` role at :1255-1256 and all four shapes at
// :1288-1291, verbatim. This file is that declaration moved into the contract module so the types can
// be generated from it. The NAMES, the FIELD SETS and the `@go(-)` tables are DESIGN.cue's.
//
// WHY IT IS A METHOD AND NOT A FIELD. DESIGN.md:413 puts a binding in the core -> plugin direction:
//
//	| binding | core -> plugin | a need or use paired with its provide, delivered by `bind` to the
//	|         | the owner of the capability type (§8) | the type owner's manifest; the nodes'
//	|         |    | provide:, need: and use:
//
// so `bind` is a WIRE method served by the capability type's owner, never an authored key. D-CAP-4 says
// the same from the other side: *"Bindings are state, delivered by the type's `bind`; they never appear
// in an authored file."* There is therefore no `bind:` key on a candy, a box or a deploy — the shapes
// below are what crosses the wire.
//
// WHAT DECIDES THE BINDING IS NOT HERE. `spec/needsolver` is D-CAP-3's one solver: it answers WHICH
// provide a need is bound to, by §8.2's visibility. This file carries what is then HANDED to the type
// owner. Deciding and delivering are two acts with two owners.

// #BindNeed — WHAT is wanted. DESIGN.cue:1289.
//
// `config` is `#Opaque` because only the capability type knows its own configuration shape (D-CAP-2:
// each type has exactly one owner). A solver that inspected it would be a second authority over a
// type it does not own.
#BindNeed: {
	type!:   #Word
	name!:   #Text
	config!: #Opaque // S10 — foreign-owned: only its type knows the shape (D-CAP-2)
}

// #BindInput — the `bind` method's input. DESIGN.cue:1288.
//
// `provided_by` names the provide that WON, and `deployment` names where the need was authored. Both are
// `#Ref` and not `#Text` because each names a node that exists in the resolved tree — the type owner
// receives references it can resolve, not free text it would have to parse.
#BindInput: {
	need!:        #BindNeed
	provided_by!: #Ref
	deployment!:  #Ref
}

// #BindOutput — what the type owner returns. DESIGN.cue:1290.
//
// `binding` is the HANDLE the type owner mints for this pairing: opaque to core, and the only thing a
// later `unbind` can name. Core never inspects it — that is what makes the type the owner of its own
// capability rather than a lookup table core reads.
#BindOutput: {
	binding!: #Opaque // S10 — foreign-owned: core never inspects the owner's handle (D-CAP-2)
}

// #UnbindInput — the `unbind` method's input. DESIGN.cue:1291.
//
// It takes the HANDLE, not the need: by the time anything is unbound, the pairing it came from may be
// gone, and the type owner is the only party that can map a handle back to its state.
#UnbindInput: {
	binding!: #Opaque // S10 — foreign-owned: only the type owner can map it back (D-CAP-2)
}

// ── the primitives these shapes name ──
//
// #Word, #Ref and #Text are already declared in this package (audit.cue, stage.cue) and are reused
// rather than re-spelled: a port that redefines a primitive is not a port. #Opaque is DESIGN.cue:93's
// `{...}` — a shape that is deliberately undecided here because its authority is the type's owner.
#Opaque: {...} @go(Opaque,type=map[string]any)
