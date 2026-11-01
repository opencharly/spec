package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDetachedDocComments is the control for the finding this repository's review raised twice on
// opencharly/spec#219: a `//` doc block separated from the `func` below it by a blank line is NOT that
// function's doc — Go attaches nothing, and the prose ends up describing whichever declaration the
// parser does bind it to (or nothing at all). gofmt and go vet do not check comment attachment, which
// is why this exists as its own gate.
//
// It checks the DETACHED class only, and deliberately does not require every func to carry a doc:
// unexported helpers and tests commonly do not, and a rule that flags them would be noise.
func TestNoDetachedDocComments(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		lines := strings.Split(string(raw), "\n")
		for i := 1; i < len(lines)-1; i++ {
			if lines[i] != "" {
				continue
			}
			if !strings.HasPrefix(lines[i-1], "//") {
				continue
			}
			if !strings.HasPrefix(lines[i+1], "func ") {
				continue
			}
			t.Errorf("%s:%d: doc comment detached from the func below it by a blank line — Go will not attach it", f, i+1)
		}
	}
}
