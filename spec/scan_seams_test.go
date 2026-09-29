package spec

import (
	"fmt"
	"strings"
	"testing"
)

// The Diag seam exists so scan diagnostics can be DATA rather than stderr writes. Before it,
// candy-version skew and local-shadow notes were `fmt.Fprintf(os.Stderr, ...)` calls, which
// made them uncountable — `charly box validate` could not report how many warnings a run
// produced, and an early draft of its summary printed "0 warnings" on a run that had just
// emitted two.
//
// It ALSO carries the LEVEL as its first argument, so a resolvable multi-tag skew can be
// reported as INFO (visible, fixable, non-gating) while only a genuinely unresolvable set is a
// WARNING. One sink, one formatter — no second channel to drift.
//
// This guards the contract the consumers rely on: the field is part of ScanSeams, it carries a
// level plus a printf-style signature, and a value set on the struct is the one that gets
// invoked.
func TestScanSeamsDiagIsALeveledPrintfSinkThatRoundTrips(t *testing.T) {
	var levels []DiagLevel
	var got []string
	seams := ScanSeams{
		Diag: func(level DiagLevel, format string, args ...any) {
			levels = append(levels, level)
			got = append(got, fmt.Sprintf(format, args...))
		},
	}
	if seams.Diag == nil {
		t.Fatal("Diag must survive being set on the struct")
	}
	seams.Diag(DiagInfo, "candy %s resolved to multiple git tags; using newest %s", "acme/thing", "2026.242.1655")
	if len(got) != 1 || len(levels) != 1 {
		t.Fatalf("expected the sink to be invoked once, got %d/%d", len(got), len(levels))
	}
	if levels[0] != DiagInfo {
		t.Errorf("the sink must receive the level, got %q", levels[0])
	}
	if !strings.Contains(got[0], "acme/thing") || !strings.Contains(got[0], "2026.242.1655") {
		t.Errorf("the sink must receive formatted arguments, got %q", got[0])
	}
}

// The two levels must be DISTINCT values: a consumer reduces them differently (INFO never
// gates, WARNING may), so collapsing them would silently re-introduce the defect.
func TestDiagLevelVocabularyIsDistinct(t *testing.T) {
	if DiagInfo == DiagWarning {
		t.Fatalf("INFO and WARNING must be distinct levels, both are %q", DiagInfo)
	}
}

// nil is a legal value and MUST stay legal: it is how a caller selects stderr explicitly.
// A consumer that assumed non-nil would panic on every existing build path.
func TestScanSeamsDiagMayBeNil(t *testing.T) {
	seams := ScanSeams{}
	if seams.Diag != nil {
		t.Errorf("the zero value must be nil, so callers can select stderr by passing nil")
	}
}
