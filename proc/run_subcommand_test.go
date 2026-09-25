package proc

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestMergedChildEnv_RunEnvOverridesAndInherits pins the ctx-carried RunEnv merge:
// a per-call key REPLACES the inherited value and a new key is added, so a bed
// member child receives the bed's OWN CHARLY_DEPLOY_CONFIG (never the operator's
// real overlay). The defect this closes: RunCharlySubcommand inherited the parent
// env verbatim, so once bed isolation retired os.Setenv a member child resolved
// the real per-host overlay and wrote the operator's state (measured live).
func TestMergedChildEnv_RunEnvOverridesAndInherits(t *testing.T) {
	t.Setenv("PROC_MERGE_OVR", "inherited")
	t.Setenv("PROC_MERGE_KEEP", "kept")

	got := mergedChildEnv(spec.RunEnv{
		"PROC_MERGE_OVR": "per-call",
		"PROC_MERGE_ADD": "added",
	})
	env := map[string]string{}
	for _, kv := range got {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	if env["PROC_MERGE_OVR"] != "per-call" {
		t.Fatalf("override = %q, want per-call", env["PROC_MERGE_OVR"])
	}
	if env["PROC_MERGE_ADD"] != "added" {
		t.Fatalf("added = %q, want added", env["PROC_MERGE_ADD"])
	}
	if env["PROC_MERGE_KEEP"] != "kept" {
		t.Fatalf("inherited key lost: %q", env["PROC_MERGE_KEEP"])
	}
	// No duplicate key — a child that saw two values could resolve either.
	n := 0
	for _, kv := range got {
		if strings.HasPrefix(kv, "PROC_MERGE_OVR=") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("PROC_MERGE_OVR appears %d times, want 1", n)
	}
}

// TestMergedChildEnv_EmptyIsInherit proves the operator path is byte-identical to
// the pre-RunEnv behavior: a nil/empty RunEnv yields a nil child env, so os/exec
// inherits the parent environment unchanged.
func TestMergedChildEnv_EmptyIsInherit(t *testing.T) {
	if mergedChildEnv(nil) != nil {
		t.Fatal("nil RunEnv should inherit (nil child env)")
	}
	if mergedChildEnv(spec.RunEnv{}) != nil {
		t.Fatal("empty RunEnv should inherit (nil child env)")
	}
}

// TestRunCharlySubcommandCtx_ThreadsRunEnv proves the exec wiring hands the ctx
// RunEnv to a REAL child process, and that the legacy no-ctx form inherits. It runs
// /bin/sh printing one variable as the child (so no charly binary is needed) via the
// testable runCharlySubcommandCtx body.
func TestRunCharlySubcommandCtx_ThreadsRunEnv(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh unavailable")
	}
	// A per-call RunEnv value must reach the child.
	ctx := spec.WithRunEnv(context.Background(), spec.RunEnv{"CHARLY_PROC_TEST": "per-call"})
	out := captureRun(t, ctx, "/bin/sh", []string{"-c", "printf %s \"$CHARLY_PROC_TEST\""})
	if out != "per-call" {
		t.Fatalf("child saw CHARLY_PROC_TEST=%q, want per-call (ctx RunEnv not threaded)", out)
	}
	// An inherited value is overridden by the per-call one.
	t.Setenv("CHARLY_PROC_TEST", "inherited")
	out = captureRun(t, ctx, "/bin/sh", []string{"-c", "printf %s \"$CHARLY_PROC_TEST\""})
	if out != "per-call" {
		t.Fatalf("child saw CHARLY_PROC_TEST=%q, want per-call override", out)
	}
	// No RunEnv → os/exec inherits the parent env unchanged.
	t.Setenv("CHARLY_PROC_TEST", "inherited-only")
	out = captureRun(t, context.Background(), "/bin/sh", []string{"-c", "printf %s \"$CHARLY_PROC_TEST\""})
	if out != "inherited-only" {
		t.Fatalf("child saw CHARLY_PROC_TEST=%q, want inherited-only (no-ctx must inherit)", out)
	}
}

// captureRun runs runCharlySubcommandCtx's wiring against /bin/sh with stdout
// redirected to a pipe (the production body streams to os.Stdout).
func captureRun(t *testing.T, ctx context.Context, exe string, args []string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := runCharlySubcommandCtx(ctx, exe, args)
	os.Stdout = old
	_ = w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if runErr != nil {
		t.Fatalf("run %s %v: %v", exe, args, runErr)
	}
	return string(buf[:n])
}
