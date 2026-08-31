package spec

import (
	"fmt"
	"strings"
	"testing"
)

// The Warn seam exists so scan advisories can be DATA rather than stderr writes. Before it,
// candy-version skew and local-shadow notes were `fmt.Fprintf(os.Stderr, ...)` calls, which
// made them uncountable — `charly box validate` could not report how many warnings a run
// produced, and an early draft of its summary printed "0 warnings" on a run that had just
// emitted two.
//
// This guards the contract the consumers rely on: the field is part of ScanSeams, it carries a
// printf-style signature, and a value set on the struct is the one that gets invoked.
func TestScanSeamsWarnIsAPrintfSinkThatRoundTrips(t *testing.T) {
	var got []string
	seams := ScanSeams{
		Warn: func(format string, args ...any) {
			got = append(got, fmt.Sprintf(format, args...))
		},
	}
	if seams.Warn == nil {
		t.Fatal("Warn must survive being set on the struct")
	}
	seams.Warn("candy %s resolved to multiple versions; using newest %s", "acme/thing", "2026.242.1655")
	if len(got) != 1 {
		t.Fatalf("expected the sink to be invoked once, got %d", len(got))
	}
	if !strings.Contains(got[0], "acme/thing") || !strings.Contains(got[0], "2026.242.1655") {
		t.Errorf("the sink must receive formatted arguments, got %q", got[0])
	}
}

// nil is a legal value and MUST stay legal: it is how a caller selects stderr explicitly.
// A consumer that assumed non-nil would panic on every existing build path.
func TestScanSeamsWarnMayBeNil(t *testing.T) {
	seams := ScanSeams{}
	if seams.Warn != nil {
		t.Errorf("the zero value must be nil, so callers can select stderr by passing nil")
	}
}
