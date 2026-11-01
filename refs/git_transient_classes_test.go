package refs

// git_transient_classes_test.go — the vocabulary's accountability to the
// RECORDED signatures.
//
// The BEHAVIOUR is asserted in git_transient_signatures_test.go, against the
// stderr strings git has actually produced. This file asserts the other
// direction: the vocabulary in refs/git.go may not carry a fault class that no
// record supports. A class with no recorded signature is a guess about a fault
// nobody has seen — and a guess is exactly how curl's "Error in the HTTP2
// framing layer" stayed out of the vocabulary and took a whole corpus
// regeneration down with it (opencharly/spec#284).
//
// The class NAME is also part of the contract: it is what the failure reports,
// so a recorded fault must be reported under the class its record names.

import (
	"strings"
	"testing"
)

// TestRecordedTransientFaultsAreClassifiedUnderTheirOwnClass: every recorded
// transient fault is classified transient, AND under the class its record
// names — so the name a reader sees is the name the record carries.
func TestRecordedTransientFaultsAreClassifiedUnderTheirOwnClass(t *testing.T) {
	for _, f := range recordedTransientFaults {
		class, transient := transientFaultClass(f.stderr)
		if !transient {
			t.Errorf("class %q: the recorded stderr %q is NOT classified transient (seen in: %s) — a fault of this class is given up on instead of retried",
				f.class, f.stderr, f.seen)
			continue
		}
		if class != f.class {
			t.Errorf("class %q: the recorded stderr %q is classified as %q — a fault must be reported under the class its record names",
				f.class, f.stderr, class)
		}
	}
}

// TestRecordedPermanentFailuresAreNotClassifiedTransient: every recorded
// permanent failure stays permanent, so it is never retried and its real error
// is never masked.
func TestRecordedPermanentFailuresAreNotClassifiedTransient(t *testing.T) {
	for _, f := range recordedPermanentFailures {
		if class, transient := transientFaultClass(f.stderr); transient {
			t.Errorf("the recorded permanent failure %q is classified as the transient class %q (seen in: %s) — a permanent failure must fail fast with one attempt",
				f.stderr, class, f.seen)
		}
	}
}

// TestEveryTransientClassHasARecordedSignature: every class in the vocabulary
// is backed by a RECORDED signature in git_transient_signatures_test.go. The
// vocabulary is accountable to what was observed — a class with no record is a
// guess, and the guess fails here rather than in a production outage.
func TestEveryTransientClassHasARecordedSignature(t *testing.T) {
	recorded := make(map[string]bool, len(recordedTransientFaults))
	for _, f := range recordedTransientFaults {
		recorded[f.class] = true
	}
	for _, sig := range gitOpTransientSignatures {
		if !recorded[sig.class] {
			t.Errorf("the transient class %q has NO recorded signature in git_transient_signatures_test.go — every class in the vocabulary must be backed by an observed stderr string",
				sig.class)
		}
	}
}

// TestTransientFailureNamesItsClass: a transient failure REPORTS the class it
// carried, so the reader learns what killed the op without reading the
// vocabulary. The class name is load-bearing in production, not decoration for
// a test to match on.
func TestTransientFailureNamesItsClass(t *testing.T) {
	useFastGitOp(t)
	dir := t.TempDir()
	installFakeGitAt(t, dir, "fatal: unable to access 'https://github.com/opencharly/x.git/': Error in the HTTP2 framing layer")
	_, err := runGitOp(false, "", nil, "ls-remote", "https://github.com/opencharly/x.git", "v1")
	if err == nil {
		t.Fatal("the fake git failed, so runGitOp must report an error")
	}
	if !strings.Contains(err.Error(), "transient network fault: http2 protocol fault") {
		t.Errorf("the failure does not name its fault class: %v", err)
	}
}
