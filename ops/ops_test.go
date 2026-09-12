package ops

import (
	"encoding/json"
	"testing"

	pb "github.com/opencharly/spec/proto"
	"github.com/opencharly/spec/spec"
)

func TestResultJSONShape(t *testing.T) {
	r, err := ResultJSON("pass", "ok")
	if err != nil {
		t.Fatal(err)
	}
	var w resultWire
	if err := json.Unmarshal(r.GetResultJson(), &w); err != nil {
		t.Fatal(err)
	}
	if w.Status != "pass" || w.Message != "ok" {
		t.Fatalf("resultWire = %+v, want {pass ok}", w)
	}
}

func TestResultJSONReturnsInvokeReply(t *testing.T) {
	r, err := ResultJSON("fail", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := interface{}(r).(*pb.InvokeReply); !ok {
		t.Fatalf("ResultJSON returned %T, want *pb.InvokeReply", r)
	}
}

func TestOpSelectorsStable(t *testing.T) {
	// These values are the wire contract — a plugin compares req.GetOp() against them.
	// Drift breaks every out-of-process plugin, so pin them.
	cases := map[string]string{
		OpRun:                "run",
		OpLoad:               "load",
		OpValidate:           "validate",
		OpEmit:               "emit",
		OpExecute:            "execute",
		OpResolve:            "resolve",
		OpBuild:              "build",
		OpCompile:            "compile",
		OpCollectContext:     "collect-context",
		OpReverse:            "reverse",
		OpPrepareVenue:       "prepare-venue",
		OpArtifactKey:        "artifact-key",
		OpPostApply:          "post-apply",
		OpTeardownExecutor:   "teardown-executor",
		OpPostTeardown:       "post-teardown",
		OpStart:              "start",
		OpStop:               "stop",
		OpStatus:             "status",
		OpLogs:               "logs",
		OpShell:              "shell",
		OpAttach:             "attach",
		OpRebuild:            "rebuild",
		OpConfigWrite:        "config-write",
		OpConfigSetup:        "config-setup",
		OpConfigRemove:       "config-remove",
		OpStatusCollect:      "status-collect",
		OpStatusCollectAll:   "status-collect-all",
		OpPreresolve:         "preresolve",
		OpBootstrap:          "bootstrap",
		OpPreflight:          "preflight",
		OpEphemeralRegister:  "ephemeral-register",
		OpEphemeralTeardown:  "ephemeral-teardown",
		OpDeployDispatch:     "deploy-dispatch",
		OpVerifyChecks:       "verify-checks",
		EphemeralPanicMarker: "ephemeral op panic:",
	}
	for sym, want := range cases {
		if sym != want {
			t.Fatalf("selector drift: got %q, want %q", sym, want)
		}
	}
}

func TestOpSelectorsDistinct(t *testing.T) {
	all := []string{
		OpRun, OpLoad, OpValidate, OpEmit, OpExecute, OpResolve, OpBuild, OpCompile,
		OpCollectContext, OpReverse, OpPrepareVenue, OpArtifactKey, OpPostApply,
		OpTeardownExecutor, OpPostTeardown, OpStart, OpStop, OpStatus, OpLogs, OpShell,
		OpAttach, OpRebuild, OpConfigWrite, OpConfigSetup, OpConfigRemove,
		OpStatusCollect, OpStatusCollectAll, OpPreresolve, OpBootstrap, OpPreflight,
		OpEphemeralRegister, OpEphemeralTeardown, OpDeployDispatch, OpVerifyChecks,
	}
	seen := map[string]bool{}
	for _, s := range all {
		if seen[s] {
			t.Fatalf("selector %q duplicated", s)
		}
		seen[s] = true
	}
}

func TestInvokeProviderOptsZero(t *testing.T) {
	var o InvokeProviderOpts
	if o.VenueDescriptor != nil {
		t.Fatal("zero VenueDescriptor should be nil")
	}
	if o.ExtraRef != "" {
		t.Fatalf("zero ExtraRef = %q, want empty", o.ExtraRef)
	}
}

func TestInvokeProviderOptsSet(t *testing.T) {
	o := InvokeProviderOpts{VenueDescriptor: &spec.VenueDescriptor{}, ExtraRef: "github.com/x/y@v1"}
	if o.VenueDescriptor == nil {
		t.Fatal("VenueDescriptor not set")
	}
	if o.ExtraRef != "github.com/x/y@v1" {
		t.Fatalf("ExtraRef = %q", o.ExtraRef)
	}
}

// ParseResultJSON is the decoder beside the ResultJSON encoder, so the round-trip IS the contract.
func TestParseResultJSONRoundTrip(t *testing.T) {
	r, err := ResultJSON("pass", "wrote 67 bytes to /tmp/screencap.png")
	if err != nil {
		t.Fatal(err)
	}
	status, message, err := ParseResultJSON(r)
	if err != nil {
		t.Fatalf("ParseResultJSON: %v", err)
	}
	if status != "pass" || message != "wrote 67 bytes to /tmp/screencap.png" {
		t.Fatalf("round-trip = (%q, %q)", status, message)
	}
	// The NON-pass direction is the one that gates a consumer's tail, so it round-trips too.
	rf, _ := ResultJSON("fail", "adb: screencap: dimensions 1080x2424 < required min 4000x4000")
	if s, m, _ := ParseResultJSON(rf); s != "fail" || m == "" {
		t.Fatalf("fail round-trip = (%q, %q)", s, m)
	}
}

// The empty cases are explicit: NO result is not a decode FAILURE — only malformed JSON is.
func TestParseResultJSONEmptyAndMalformed(t *testing.T) {
	if s, m, err := ParseResultJSON(nil); s != "" || m != "" || err != nil {
		t.Fatalf("nil reply = (%q, %q, %v), want empty and no error", s, m, err)
	}
	if s, m, err := ParseResultJSON(&pb.InvokeReply{}); s != "" || m != "" || err != nil {
		t.Fatalf("empty reply = (%q, %q, %v), want empty and no error", s, m, err)
	}
	if _, _, err := ParseResultJSON(&pb.InvokeReply{ResultJson: []byte("not-json")}); err == nil {
		t.Fatal("a malformed payload must return a non-nil error")
	}
}
