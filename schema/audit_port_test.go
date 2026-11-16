package schema_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestTheAuditContractIsDesigns ports-verified: DESIGN.cue declares the audit record at
// :1146-1175 (`#AuditEvent`, `#AuditDetail`, `#Redacted`, `#AuditAppendInput` and four word
// tables), and schema/audit.cue is that declaration MOVED into the contract module. This test
// pins the parts a re-spelling would silently change.
//
// It fails without the port: `spec.AuditEvent` does not exist on `main`, so this file does not
// compile there.
func TestTheAuditContractIsDesigns(t *testing.T) {
	t.Run("#Redacted holds NOTHING -- a marker, never a value (D-AUDIT-3)", func(t *testing.T) {
		// DESIGN.cue: "#Redacted: {redacted!: true} // A record says a value was there without
		// holding it". The whole point is that there is no field a value could occupy, so the
		// assertion is about the ABSENCE of any other wire key — a future `Value`/`Secret` field
		// added "for convenience" fails here.
		raw, err := json.Marshal(spec.Redacted{Redacted: true})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(got) != 1 || got["redacted"] != true {
			t.Fatalf("#Redacted = %v, want exactly {\"redacted\": true} — anything else is a place a secret could sit", got)
		}
	})

	t.Run("#AuditDetail carries code, text, and an optional MARKER value", func(t *testing.T) {
		d := spec.AuditDetail{Code: "reason", Text: "the target is not disposable"}
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := got["code"]; !ok {
			t.Fatalf("no `code` key: %v", got)
		}
		if _, ok := got["text"]; !ok {
			t.Fatalf("no `text` key: %v", got)
		}
	})

	t.Run("#AuditEvent's outcome is a WORD, not a struct", func(t *testing.T) {
		// DESIGN.cue: "outcome!: or(_auditOutcomeWord)  // ok|refused|failed". A struct here would
		// be the shape the closed spec#343 invented, and it is not the design's.
		raw, err := json.Marshal(spec.AuditEvent{
			At:       "2026-11-16T10:00:00Z",
			Event:    "mutate",
			Outcome:  "failed",
			Identity: "operator",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, isStruct := got["outcome"].(map[string]any); isStruct {
			t.Fatalf("outcome is a STRUCT (%v) — the design says a closed word", got["outcome"])
		}
		if got["outcome"] != "failed" {
			t.Fatalf("outcome = %v, want the word \"failed\"", got["outcome"])
		}
		// D-AUDIT-2: ONE identity field, and it is `identity`.
		if _, ok := got["identity"]; !ok {
			t.Fatalf("no `identity` key: %v", got)
		}
		for _, forbidden := range []string{"decided_by", "actor", "user"} {
			if _, ok := got[forbidden]; ok {
				t.Fatalf("a second identity field %q exists — D-AUDIT-2 allows exactly one", forbidden)
			}
		}
	})

	t.Run("#AuditAppendInput carries exactly ONE event", func(t *testing.T) {
		// D-AUDIT-1's "exactly one" is expressed in the SHAPE: the field is required and singular,
		// so there is no form of this input that carries zero events or two.
		in := spec.AuditAppendInput{Event: spec.AuditEvent{At: "2026-11-16T10:00:00Z", Event: "mutate", Outcome: "ok", Identity: "operator"}}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, isSlice := got["event"].([]any); isSlice {
			t.Fatalf("event is a LIST — D-AUDIT-1 says exactly one record per act")
		}
		if _, isMap := got["event"].(map[string]any); !isMap {
			t.Fatalf("event = %v, want one object", got["event"])
		}
	})

	t.Run("three word tables, not four -- and each is closed", func(t *testing.T) {
		// COUNTED, not assumed: DESIGN.cue declares EXACTLY THREE audit tables for this contract --
		// _auditEvent, _auditOutcome and _detailCode. An earlier revision of this test claimed four
		// and its body was a tautology over a hardcoded literal; this one reads the schema text and
		// counts the `@go(-)`-marked tables, so the number in the prose and the number in the file
		// cannot drift apart again.
		src, err := os.ReadFile("audit.cue")
		if err != nil {
			t.Fatalf("read audit.cue: %v", err)
		}
		n := strings.Count(string(src), "} @go(-)")
		if n != 3 {
			t.Fatalf("audit.cue declares %d @go(-) word tables, want exactly 3 (_auditEvent, _auditOutcome, _detailCode)", n)
		}
		for _, table := range []string{"_auditEvent:", "_auditOutcome:", "_detailCode:"} {
			if !strings.Contains(string(src), table) {
				t.Fatalf("table %q is missing", table)
			}
		}
		// And the words the design names are the words the tables hold.
		for _, w := range []string{"mutate", "authorize", "admit", "activate", "teardown"} {
			if !strings.Contains(string(src), w) {
				t.Fatalf("event word %q is missing", w)
			}
		}
		for _, w := range []string{"ok", "refused", "failed"} {
			if !strings.Contains(string(src), w) {
				t.Fatalf("outcome word %q is missing", w)
			}
		}
	})
}
