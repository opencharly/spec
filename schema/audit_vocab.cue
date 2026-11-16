package schema

// audit_vocab.cue — the audit record (D-AUDIT) and the closed host-method vocabulary (§4.6).
//
// DESIGN.md §20.1 names this as the ONE requirement area whose subject was genuinely absent:
//
//   "The only requirement area whose subject was genuinely absent: charly logged, but kept no
//    append-only record of an action and its outcome, and — like OCE, whose own RFC-0013 is
//    unimplemented — had no read path for one. Records live in the state root (D-DEP-5), so they
//    inherit its durability story."
//
// So this file is the CONTRACT half of the absence: the record's shape and the method set that
// writes one. The writer lives in the sdk; the caller wiring in charly.
//
// Every field below exists because a numbered row requires it. A field with no row behind it is a
// field this file should not have, and a row with no field is a requirement it fails to express.

// -----------------------------------------------------------------------------
// §4.6 — the host service is CLOSED: "adding a method amends this document".
//
// The eight methods are the whole vocabulary a plugin may reach. Writing them as a closed
// disjunction is what makes §4.6's "closed" checkable rather than aspirational: a plugin naming a
// ninth method cannot unify, so the addition has to happen HERE, where it is visible in the diff.
//
// @go(-): the disjunction itself projects no type. What projects is the CLOSED LIST below
// (#HostMethods -> spec.HostMethods, derived from this def), because a closed set that nothing can
// enumerate is closed only by convention: a host dispatching on it, and a test asserting its
// membership, both need the list rather than the spelling of each member.
_hostMethodWord:
	"venue_exec" |
	"get" |
	"put" |
	"project" |
	"call" |
	"report" |
	"state_commit" |
	"audit_append"

// NO enumerable list here, and that is a measured decision rather than an omission: schemagen
// projects a CLOSED list as `[]any /* CUE closed list */`, and the only `[]string` vocabularies in
// spec/vocab_gen.go (#DistroIDs and friends) are SPECIAL-CASED extractions in
// internal/schemagen/params.go against a map-shaped table. A per-method map (as #Distros is) would
// give both a closed set and a derived list — that is the follow-up unit, filed rather than
// faked here with a []any nobody can dispatch on.

// §4.6 permits each method "always" or under a named condition. The condition is part of the
// method's identity: `audit_append` is permitted ALWAYS because a refused act is exactly the one
// that most needs a record (D-AUDIT-1 covers "every authority decision", including a denied one).
#HostMethod: {
	// name is the method word itself. The disjunction is INLINED rather than `or(_hostMethodWord)`:
	// measured, `or()` over a string disjunction fails composition with
	// `cannot use "venue_exec" | … (type string) as list` — the same deviation recorded in
	// schema/stage.cue. The closed set is still single-sourced, two lines up.
	name: "venue_exec" | "get" | "put" | "project" | "call" | "report" | "state_commit" | "audit_append"
	// permitted_when is the condition §4.6's right column states.
	permitted_when: "always" | "a deployment's venue" | "the call's project" | "the caller's plugin candy lists that api in need" | "the caller's plugin owns that deployment's kind"
} @go(-)

// -----------------------------------------------------------------------------
// §20.1 — the audit record.
//
// D-AUDIT-3: "#Redacted, because an audit that holds a secret is a leak with a timestamp."
//
// A redacted reference is a NAME, never a value: the record may say that a secret was used, and
// which one, but never what it held. Making it a distinct type (not a string that happens to look
// like one) is the point — a caller cannot accidentally hand a value to a field typed #Redacted
// without writing `secret:` in front of it.
#Redacted: {
	// secret names the secret that was referenced. It is the name, not the value.
	secret: string
} @go(Redacted)

// #AuditOutcome is what the act produced. D-AUDIT-1 requires a record for an act that FAILED as
// well as one that succeeded — an append-only record of successes only is a record that cannot
// explain an incident.
#AuditOutcome: {
	// ok is whether the act succeeded.
	ok: bool
	// message carries the failure's reason when ok is false, and is absent otherwise. A message
	// on a success is noise, and noise in an audit is how a real finding hides.
	message?: string
} @go(AuditOutcome)

// #AuditRecord is ONE act and its outcome. It is append-only by construction (D-AUDIT-4): this
// type has no field naming an earlier record, so a record cannot reference a record it amends.
#AuditRecord: {
	// identity is WHO acted (D-AUDIT-2). ONE field for both a mutation and an authority decision —
	// "authority is the grant of a need (§8), not a separate concept", so a separate
	// `decided_by:` field would be the duplication D-AUDIT-2 forbids.
	identity: string

	// action is a charly verb or mutation word (D-AUDIT-5), never a foreign system's own verb.
	// Its value set is the verb vocabulary; this field does not accept free text by intent, and
	// the writer validates it against the verbs it knows.
	action: string

	// outcome is what the act produced — required, because D-AUDIT-1's record is of "an action and
	// its outcome" and an outcome-less record cannot answer the question an audit exists for.
	outcome: #AuditOutcome

	// redacted lists any secret the act referenced, BY NAME (D-AUDIT-3). Absent when the act
	// referenced none; never a value.
	redacted?: [...#Redacted]
} @go(AuditRecord)
