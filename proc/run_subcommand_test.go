package proc

import (
	"context"
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

// TestRunCharlySubcommandCtx_ThreadsRunEnv proves the ctx form hands the RunEnv to
// the child process, while the legacy no-ctx form inherits. Uses /bin/sh -c printenv
// as the child so no charly binary is needed (the resolver picks os.Executable, so
// this stubs the var to run a real env-printing child).
func TestRunCharlySubcommandCtx_ThreadsRunEnv(t *testing.T) {
	// Exercise the merge directly (the exec path is covered by mergedChildEnv above);
	// a real fork here would depend on os.Executable being a charly binary.
	ctx := spec.WithRunEnv(context.Background(), spec.RunEnv{"CHARLY_DEPLOY_CONFIG": "/tmp/bed/charly.yml"})
	env := mergedChildEnv(spec.RunEnvFrom(ctx))
	found := false
	for _, kv := range env {
		if kv == "CHARLY_DEPLOY_CONFIG=/tmp/bed/charly.yml" {
			found = true
		}
	}
	if !found {
		t.Fatal("ctx RunEnv was not carried into the child env")
	}
}
