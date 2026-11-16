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
// §4.6 — the host service is CLOSED, and its method set is NOT declared here. Measured twice, and
// both measurements say this file is the wrong place for it:
//
//   1. A CUE def carrying the 8-member disjunction projects NOTHING usable: `@go(-)` gives no Go
//      value to assert membership on, and without it a closed list projects as
//      `[]any /* CUE closed list */`, which no dispatcher can range over. A "closed set" nothing can
//      enumerate is closed by convention, not by construction — and a test that cannot reference it
//      is a test that proves nothing.
//   2. Writing the disjunction twice (a named def AND the inlined field it feeds) is the
//      duplication R2 forbids, and the comment claiming "single-sourced" would be false.
//
// So the method set stays where DESIGN.md puts it — §4.6's table, which states that adding a method
// "amends this document". `audit_append` is a member there. Giving that table a checkable Go form
// needs the map-shaped-vocabulary extraction that #DistroIDs has (internal/schemagen special-cases
// it against a map table), which is a unit of its own — filed, not faked.

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
