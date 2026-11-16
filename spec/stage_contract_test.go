package spec

import (
	"encoding/json"
	"testing"
)

// TestTheStageVocabularyCarriesDesignsShape pins P26's contract defs against DESIGN.cue:894-915.
//
// It fails without the change (the types do not exist), and it fails if the shape drifts in the ways
// that matter: a stage carries its ORDERING EDGE (`after:`) and its concurrency bound, the four arms
// are distinct, and a retry is bounded by `retry.attempt:` — the level row 2.22 gets wrong.
func TestTheStageVocabularyCarriesDesignsShape(t *testing.T) {
	// The exact shape charly#1168 needs the roster to become:
	//   stage: {<bed>: {deployment: [...], after: [...producers], concurrency: N}}
	raw := `{"deployment":{"deployment":["bed-a"]},"after":["bed-base"],"concurrency":4}`
	var st Stage
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatalf("a deployment stage with an after: list and a concurrency does not decode: %v", err)
	}
	if got := st.Deployment.Deployment; len(got) != 1 || got[0] != "bed-a" {
		t.Fatalf("deployment arm = %v, want [bed-a]", got)
	}
	if got := st.After; len(got) != 1 || got[0] != "bed-base" {
		t.Fatalf("after: = %v, want the producer bed — the ordering edge is what removes the race", got)
	}
	if st.Concurrency != 4 {
		t.Fatalf("concurrency = %d, want 4 — the bound the roster's lanes used to express", st.Concurrency)
	}

	// A stage does EXACTLY one thing (D-FLOW-1): the arms are distinct fields of one sum.
	var stepStage Stage
	if err := json.Unmarshal([]byte(`{"step":{"step":[{"check":"the artifact runs"}]}}`), &stepStage); err != nil {
		t.Fatalf("the step arm does not decode: %v", err)
	}
	if len(stepStage.Step.Step) != 1 {
		t.Fatalf("step arm decoded %d steps, want 1", len(stepStage.Step.Step))
	}
	if stepStage.Deployment.Deployment != nil || stepStage.Ask.Prompt != "" {
		t.Fatal("a step stage must carry no other arm")
	}

	var askStage Stage
	if err := json.Unmarshal([]byte(`{"ask":{"prompt":"deploy?","answer":{"unanswered":"refused"}}}`), &askStage); err != nil {
		t.Fatalf("the ask arm does not decode: %v", err)
	}
	if askStage.Ask.Prompt != "deploy?" {
		t.Fatalf("the ask arm lost its prompt: %+v", askStage.Ask)
	}
	// An EXPLICIT answered outcome round-trips. The `*"refused"` default is a CUE-layer fact: Go's
	// decoder does not apply schema defaults (the same reason `retry.interval` is asserted with an
	// explicit value below), so asserting the default here would prove nothing.
	if got := askStage.Ask.Answer.Unanswered; got != "refused" {
		t.Fatalf("answer.unanswered = %q, want the authored value to round-trip", got)
	}
	var unansweredEmpty Stage
	if err := json.Unmarshal([]byte(`{"ask":{"prompt":"deploy?","answer":{}}}`), &unansweredEmpty); err != nil {
		t.Fatalf("an ask with no answered outcome does not decode: %v", err)
	}
	if got := unansweredEmpty.Ask.Answer.Unanswered; got != "" {
		t.Fatalf("answer.unanswered = %q with no authored value; the CUE default must NOT leak into Go", got)
	}

	// D-FLOW-2: repetition is a DECLARED, BOUNDED count — retry.attempt, not a top-level field.
	var r Retry
	if err := json.Unmarshal([]byte(`{"until":"30s","attempt":3}`), &r); err != nil {
		t.Fatalf("a bounded retry does not decode: %v", err)
	}
	if r.Attempt != 3 {
		t.Fatalf("retry.attempt = %d, want 3 — row 2.22 names attempt at exactly this level", r.Attempt)
	}
	// The `*"2s"` default is a CUE-layer fact: Go's decoder does not apply schema defaults, so this
	// asserts an EXPLICIT interval round-trips rather than pretending the default leaks into Go.
	var r2 Retry
	if err := json.Unmarshal([]byte(`{"interval":"5s"}`), &r2); err != nil {
		t.Fatalf("an explicit retry interval does not decode: %v", err)
	}
	if r2.Interval != "5s" {
		t.Fatalf("retry.interval = %q, want the explicit 5s", r2.Interval)
	}
}
