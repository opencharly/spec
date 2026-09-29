package spec

import "testing"

// The severity taxonomy is a CLOSED vocabulary with ONE reduction shared by every consumer. Before
// it, HasErrors asked `Severity != "warning"`, so adding the scan's resolvable INFO tier would have
// made every resolvable closure fail its gate. This pins the reduction: ONLY error-tier fails.
func TestDiagnosticsHasErrorsCountsOnlyErrorTier(t *testing.T) {
	cases := []struct {
		name     string
		severity string
		want     bool
	}{
		{"info is not an error", SeverityInfo, false},
		{"warning is not an error", SeverityWarning, false},
		{"error is an error", SeverityError, true},
		{"empty severity counts as error (producer must be explicit)", "", true},
		{"an unknown tier is treated as error", "fatal", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Diagnostics{Items: []Diagnostic{{Severity: tc.severity, Message: "x"}}}
			if got := d.HasErrors(); got != tc.want {
				t.Errorf("HasErrors(severity=%q) = %v, want %v", tc.severity, got, tc.want)
			}
		})
	}
}

// IsError is the ONE predicate; the closed vocabulary is exported so no consumer hardcodes a tier.
func TestDiagnosticIsErrorAndVocabulary(t *testing.T) {
	if SeverityInfo == SeverityWarning || SeverityWarning == SeverityError || SeverityInfo == SeverityError {
		t.Fatal("the three severity tiers must be distinct values")
	}
	if (Diagnostic{Severity: SeverityInfo}).IsError() {
		t.Error("INFO must not be an error")
	}
	if (Diagnostic{Severity: SeverityWarning}).IsError() {
		t.Error("WARNING must not be an error")
	}
	if !(Diagnostic{Severity: SeverityError}).IsError() {
		t.Error("ERROR must be an error")
	}
}
