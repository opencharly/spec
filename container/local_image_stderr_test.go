package container

// local_image_stderr_test.go — the regression guard for the opaque enumeration failure:
// `charly clean --images` reported only `listing local images via podman: exit status 1`
// while podman had a full explanation on stderr (measured 2026-11-18, with a build holding
// the engine's storage lock). The engine's text is already captured by `cmd.Output()`; the
// defect was dropping it. These tests fail if a future error path drops it again.

import (
	"strings"
	"testing"
)

// TestDefaultListLocalImages_CarriesTheEngineStderr pins the fix: a non-zero exit carries the
// engine's own stderr text, so the message names the cause instead of only the status.
func TestDefaultListLocalImages_CarriesTheEngineStderr(t *testing.T) {
	const diagnosis = "Error: database storage is locked by another process"
	fakeEngine(t, "#!/bin/sh\n"+
		"echo '"+diagnosis+"' >&2\n"+
		"exit 1\n")

	_, err := defaultListLocalImages("podman")
	if err == nil {
		t.Fatal("a failing engine must produce an error, not a nil result")
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("the error no longer names the exit status: %v", err)
	}
	if !strings.Contains(err.Error(), diagnosis) {
		t.Errorf("the error drops the engine's own stderr — the operator cannot tell what failed:\n%v", err)
	}
	if !strings.Contains(err.Error(), "podman") {
		t.Errorf("the error does not name the engine binary: %v", err)
	}
}

// TestDefaultListLocalImages_BoundsTheEngineStderr pins the bound: an engine that floods
// stderr must not flood the error message, and the truncation must be visible (an ellipsis),
// never silent.
func TestDefaultListLocalImages_BoundsTheEngineStderr(t *testing.T) {
	// 2000 'x' characters, far past the bound.
	flood := strings.Repeat("x", 2000)
	fakeEngine(t, "#!/bin/sh\n"+
		"printf '%s' '"+flood+"' >&2\n"+
		"exit 1\n")

	_, err := defaultListLocalImages("podman")
	if err == nil {
		t.Fatal("a failing engine must produce an error")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Errorf("a truncated stderr must say so with an ellipsis: %v", err)
	}
	// The detail is the tail of the message; assert the whole message stays near the bound
	// rather than carrying the whole flood.
	if len([]rune(err.Error())) > engineFailureDetailMaxRunes+200 {
		t.Errorf("the error carries %d runes — the engine stderr must be bounded to %d",
			len([]rune(err.Error())), engineFailureDetailMaxRunes)
	}
	if strings.Contains(err.Error(), strings.Repeat("x", engineFailureDetailMaxRunes+1)) {
		t.Error("the error carries more than the bound's worth of engine stderr")
	}
}

// TestDefaultListLocalImages_NoStderrStaysClean pins the empty case: a failure that carries
// no stderr (a signal, a start failure) keeps the plain wrap rather than gaining a dangling
// ": " separator.
func TestDefaultListLocalImages_NoStderrStaysClean(t *testing.T) {
	fakeEngine(t, "#!/bin/sh\nexit 1\n")

	_, err := defaultListLocalImages("podman")
	if err == nil {
		t.Fatal("a failing engine must produce an error")
	}
	if strings.HasSuffix(err.Error(), ": ") {
		t.Errorf("the error ends in a dangling separator: %q", err.Error())
	}
}
