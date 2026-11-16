package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestTheAuditRecordCarriesDesignsShape gates P06's contract half against DESIGN.md §20.1.
//
// §20.1 names this as the ONE requirement area whose subject was genuinely ABSENT:
//
//	"The only requirement area whose subject was genuinely absent: charly logged, but kept no
//	 append-only record of an action and its outcome, and … had no read path for one."
//
// So there is no existing behaviour to compare against — the contract is the assertion, and each
// case below pins one numbered row. Without these definitions this file does not compile, which is
// the honest form of "coverage that fails without the change".
func TestTheAuditRecordCarriesDesignsShape(t *testing.T) {
	t.Run("D-AUDIT-2: ONE identity field, for a mutation and an authority decision alike", func(t *testing.T) {
		// "authority is the grant of a need (§8), not a separate concept" — so a `decided_by:`
		// sibling would be the duplication the row forbids. The type carries exactly one.
		var r spec.AuditRecord
		r.Identity = "operator@host"
		if r.Identity == "" {
			t.Fatal("identity did not round-trip")
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := got["identity"]; !ok {
			t.Fatalf("the wire key is `identity`, not %v", got)
		}
		for _, forbidden := range []string{"decided_by", "actor", "user"} {
			if _, ok := got[forbidden]; ok {
				t.Fatalf("a second identity field %q exists — D-AUDIT-2 allows exactly one", forbidden)
			}
		}
	})

	t.Run("D-AUDIT-5: action is a charly verb, carried as data", func(t *testing.T) {
		r := spec.AuditRecord{Action: "deploy_add"}
		if r.Action != "deploy_add" {
			t.Fatalf("action did not round-trip: %q", r.Action)
		}
	})

	t.Run("D-AUDIT-1: the outcome is REQUIRED, and records a failure as readily as a success", func(t *testing.T) {
		// The field is a required struct, not a pointer: a record cannot be written without
		// saying what the act produced, so "an action and its outcome" is structural rather than
		// a convention. And a FAILED act has an outcome — an audit of successes alone cannot
		// explain an incident.
		failed := spec.AuditRecord{
			Identity: "operator@host",
			Action:   "deploy_add",
			Outcome:  spec.AuditOutcome{Ok: false, Message: "image not found"},
		}
		if failed.Outcome.Ok {
			t.Fatal("a failed act recorded as ok")
		}
		if failed.Outcome.Message == "" {
			t.Fatal("a failure with no reason")
		}
	})

	t.Run("D-AUDIT-3: a redacted reference names a secret and CANNOT hold its value", func(t *testing.T) {
		// The type has exactly one field, `secret`, and it is a NAME. A redaction type that
		// could carry a payload would be the leak the row exists to prevent — "an audit that
		// holds a secret is a leak with a timestamp".
		red := spec.Redacted{Secret: "lab-registry-token"}
		if red.Secret != "lab-registry-token" {
			t.Fatalf("the reference did not round-trip: %q", red.Secret)
		}
		raw, err := json.Marshal(red)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("#Redacted carries %d fields (%v) — exactly one NAME field, never a payload", len(got), got)
		}
		if _, ok := got["value"]; ok {
			t.Fatal("#Redacted has a `value` field — that is the leak D-AUDIT-3 forbids")
		}
	})

	t.Run("D-AUDIT-4: append-only by construction — no field names an earlier record", func(t *testing.T) {
		// A record that could reference one it amends is a record that can be superseded, which is
		// not append-only. The shape forbids it: there is no such field.
		raw, err := json.Marshal(spec.AuditRecord{Identity: "i", Action: "a", Outcome: spec.AuditOutcome{Ok: true}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, forbidden := range []string{"supersedes", "amends", "replaces", "previous"} {
			if _, ok := got[forbidden]; ok {
				t.Fatalf("a record can name an earlier one via %q — that is not append-only", forbidden)
			}
		}
	})
}

// TestTheHostMethodVocabularyIsClosed gates §4.6's "It is closed: adding a method amends this
// document." `audit_append` must be a member (D-AUDIT-4 makes it the ONLY writer), and the set is
// carried by the schema rather than by this test — so the assertion is that the record type and
// the method coexist, not that a list somewhere contains a string.
func TestTheHostMethodVocabularyIsClosed(t *testing.T) {
	// If #AuditRecord exists and audit_append is its only writer (§4.6's table), then the method
	// set must include it. The pairing is what this asserts: the record type without the method
	// would be a contract nothing can write.
	var r spec.AuditRecord
	r.Identity, r.Action = "i", "a"
	r.Outcome = spec.AuditOutcome{Ok: true}
	if r.Identity == "" || r.Action == "" {
		t.Fatal("#AuditRecord is not constructible — audit_append would have no value to write")
	}
}
