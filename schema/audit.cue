package schema

import "list"

// audit.cue — the audit record (§20.1), PORTED from DESIGN.cue, not re-designed.
//
// DESIGN.cue is the single schema copy (PROGRAM/design-implementation.md), and it already declares
// this contract in full at DESIGN.cue:1146-1175. This file is that declaration moved into the
// contract module so the code can be generated from it — the NAMES, the WORD SETS and the FIELD SET
// are DESIGN.cue's, verbatim. Nothing here is a new shape.
//
// An earlier attempt (opencharly/spec#343, closed) invented `#AuditRecord`/`#AuditOutcome` and a
// `#Redacted{secret: string}` beside a declared contract. It was wrong three ways — a duplicate
// name, a struct where the design says a closed word, and a `#Redacted` that HOLDS a name where the
// design holds NOTHING. That is what this file replaces.

// ── the primitives this contract names, PORTED from DESIGN.cue:50-72 and :354 ──
//
// They are not invented here: each is DESIGN.cue's own definition, byte for byte, because a port that
// re-spells a primitive is not a port. They live beside the shapes that use them rather than in a
// shared primitives file because nothing else in this module uses them yet, and a shared file with
// one consumer is a place for a second spelling to appear.
#Word:      =~"^[a-z][a-z0-9]*(_[a-z0-9]+)*$"                       // D-NAME-2: a schema word
#Ref:       =~"^[a-z][a-z0-9]*(-[a-z0-9]+)*(\\.[a-z][a-z0-9]*(-[a-z0-9]+)*)*$"
#Digest:    =~"^sha256:[0-9a-f]{64}$"
#Timestamp: =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?Z$"
#TopName:   =~"^[a-z][a-z0-9]*(-[a-z0-9]+)*$"
#Namespace: #TopName

// ── §20.1: one append-only record per mutation and per authority decision ──
//
// The THREE word sets are DESIGN.cue's, and they are TABLES rather than bare unions so the words
// carry their own existence: D-GEN-8's D1 form derives the list from the table, and a word added
// here is a schema change rather than a payload choice.
_auditEvent: {mutate: {}, authorize: {}, admit: {}, activate: {}, teardown: {}} @go(-) @vocab(spec)
_auditEventWord: [for w, _ in _auditEvent {w}]
_auditOutcome: {ok: {}, refused: {}, failed: {}} @go(-) @vocab(spec)
_auditOutcomeWord: [for w, _ in _auditOutcome {w}]
_detailCode: {reason: {}, path: {}, field: {}, ref: {}, count: {}, duration: {}} @go(-) @vocab(spec)
_detailCodeWord: [for w, _ in _detailCode {w}]

// A record says a value was there WITHOUT holding it (D-AUDIT-3): "an audit that holds a secret is a
// leak with a timestamp". The shape is a bare marker — there is no field a value could occupy, which
// is a stronger guarantee than validating a string that is supposed to be a name.
#Redacted: {redacted!: true}

// #AuditDetail is one structured item of context. `value?` is the ONLY place a value could appear,
// and its type admits nothing but the marker above.
#AuditDetail: {
	code!:  or(_detailCodeWord)
	text!:  #Text
	value?: #Redacted @go(Value,type=*Redacted)
}

// #AuditEvent is ONE act and its outcome.
#AuditEvent: {
	// at is when it happened.
	at!: #Timestamp
	// event is what kind of act it was — the five words above.
	event!: or(_auditEventWord)
	// outcome is how it ended, as a WORD: ok, refused, or failed. A refused act is recorded (D-AUDIT-1
	// covers "every authority decision", including a denied one) — an append-only record of successes
	// alone cannot explain an incident.
	outcome!: or(_auditOutcomeWord)
	// identity is WHO acted (D-AUDIT-2) — ONE field for a mutation and an authority decision alike,
	// because "authority is the grant of a need (§8), not a separate concept".
	identity!: #Ref
	// target? is what was acted on.
	target?: #Ref
	// action? is a charly verb or mutation word (D-AUDIT-5), never a foreign system's own verb.
	action?: #Word
	// namespace? is where it held.
	namespace?: #Namespace
	// revision? is the digest the act produced or addressed.
	revision?: #Digest
	// detail? is bounded: an unbounded record is an unbounded disk cost on a path every mutation
	// takes.
	detail?: [...#AuditDetail] & list.MaxItems(32)
}

// #AuditAppendInput is the host method's input (§4.6 `audit_append`): one event, exactly one.
//
// The field is REQUIRED and singular, which is how D-AUDIT-1's "exactly one record" is expressed in
// the shape: there is no form of this input that carries zero events or two.
#AuditAppendInput: {event!: #AuditEvent}
