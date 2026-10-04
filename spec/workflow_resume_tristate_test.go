package spec

// workflow_resume_tristate_test.go — the WIRE proof for #WorkflowResumeRequest.approve.
//
// WHY THIS TEST EXISTS. The field is a pointer for a WIRE reason, and the closedness
// corpus beside it (workflow_envelope_test.go) structurally cannot see that reason:
// `approve?: bool` and `approve?: bool @go(,type=*bool)` have the SAME CUE type, so
// unifying concrete values accepts and rejects identically under both. The change is
// invisible to the schema and visible only in the generated Go and in the JSON that
// leaves the host over InvokeProvider's `--request-json` — which is exactly what
// plugin-lobster's `decodeParams` reads on the other side. So this test marshals the
// request the way the dispatcher does, reads the object back, and then decodes both
// wire forms the way the engine does.
//
// R7 — this file does not COMPILE against the pre-change `Approve bool`, because there
// `false` cannot be addressed as a value distinct from the zero value at all: the
// defect stated as a compile error. With the pointer it compiles, and the three states
// a caller can mean are three different objects on the wire.

import (
	"encoding/json"
	"testing"
)

// triBoolPtr exists so a test case can spell "explicitly false" — the state the
// pre-change field could not represent.
func triBoolPtr(b bool) *bool { return &b }

// resumeWire marshals a resume request the way the host dispatches it and returns the
// resulting JSON object, so a field's PRESENCE can be judged separately from its value.
func resumeWire(t *testing.T, req WorkflowResumeRequest) map[string]any {
	t.Helper()
	raw, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("marshal resume request: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return got
}

func TestWorkflowResumeApproveIsTriState(t *testing.T) {
	// The three states, and the wire object each must produce. The middle row is the
	// one the change exists for: under the pre-change `bool` it was unreachable, and
	// it marshalled byte-identically to the first row.
	for _, tc := range []struct {
		name    string
		approve *bool
		wantKey bool
		wantVal bool
	}{
		{"no answer — the key must be ABSENT, not false", nil, false, false},
		{"false — the human REJECTED the gate", triBoolPtr(false), true, false},
		{"true — the human approved the gate", triBoolPtr(true), true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resumeWire(t, WorkflowResumeRequest{
				Pipeline: "nightly",
				Id:       "a1b2c3d4",
				Approve:  tc.approve,
			})

			raw, present := got["approve"]
			if present != tc.wantKey {
				t.Fatalf("approve present = %v, want %v (object: %v)", present, tc.wantKey, got)
			}
			if !tc.wantKey {
				return
			}
			if raw != tc.wantVal {
				t.Fatalf("approve = %v, want %v", raw, tc.wantVal)
			}
		})
	}
}

// TestWorkflowResumeApproveSurvivesTheEngineSideDecode is the other half of the same
// claim, judged where it matters: plugin-lobster decodes the wire into this type. A
// rejection must arrive as a REJECTION and no-answer must arrive as nil.
func TestWorkflowResumeApproveSurvivesTheEngineSideDecode(t *testing.T) {
	var rejected WorkflowResumeRequest
	if err := json.Unmarshal([]byte(`{"pipeline":"nightly","id":"a1b2c3d4","approve":false}`), &rejected); err != nil {
		t.Fatalf("unmarshal a rejected gate: %v", err)
	}
	if rejected.Approve == nil {
		t.Fatal(`{"approve":false} decoded to a nil approve — a rejection is indistinguishable from no answer`)
	}
	if *rejected.Approve {
		t.Fatalf("approve = true, want false")
	}

	var unanswered WorkflowResumeRequest
	if err := json.Unmarshal([]byte(`{"pipeline":"nightly","id":"a1b2c3d4"}`), &unanswered); err != nil {
		t.Fatalf("unmarshal an unanswered gate: %v", err)
	}
	if unanswered.Approve != nil {
		t.Fatalf("an absent approve decoded to %v, want nil", unanswered.Approve)
	}
}
