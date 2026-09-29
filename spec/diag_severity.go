package spec

// diag_severity.go — the CLOSED severity vocabulary for a spec.Diagnostic item.
//
// A Diagnostic's Severity was compared as a raw string literal ("warning"/"error") at every
// consumer, so a new tier could not be added without silently changing a consumer's meaning:
// `HasErrors` and the validate/build verdicts each asked "is it NOT warning?" — which made any
// third tier count as an ERROR and fail a gate that should have passed. The scan's resolvable
// multi-tag skew is exactly such a tier (INFO), so the vocabulary is declared ONCE here and every
// consumer matches on the constants. ONE string domain, ONE reduction per tier.
const (
	// SeverityInfo is a RESOLVABLE observation: the resolver had a deterministic winner, so nothing
	// is unresolved and nothing needs fixing to proceed. It is reported for visibility and MUST NOT
	// fail any gate. It is the scan seam's DiagInfo (spec.ScanSeams.Diag).
	SeverityInfo = "info"
	// SeverityWarning is a non-fatal advisory that a consumer may surface and count, but that does
	// NOT fail a gate (the zero-warnings policy is a separate, stricter reduction).
	SeverityWarning = "warning"
	// SeverityError is a genuine failure: the only tier a verdict may turn into a non-zero exit.
	SeverityError = "error"
)

// IsError reports whether a diagnostic Severity is failure-tier. An EMPTY severity counts as error
// (the established convention — a producer that meant INFO must say so), and every non-error tier
// (info, warning) does NOT. This is the ONE predicate the verdicts share, so a new tier cannot
// reintroduce the "anything not warning is an error" defect.
func (d Diagnostic) IsError() bool {
	return d.Severity != SeverityInfo && d.Severity != SeverityWarning
}
