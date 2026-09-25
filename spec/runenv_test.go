package spec

import (
	"context"
	"fmt"
	"testing"
)

// TestRunEnvGet_CtxWinsOverProcessEnv proves the per-invocation RunEnv on ctx takes
// precedence over the process env — the whole point of the primitive: a concurrent
// in-process bed roster threads each bed's own value on its ctx, so bed A's process
// env (if any) can never leak into bed B's read.
func TestRunEnvGet_CtxWinsOverProcessEnv(t *testing.T) {
	t.Setenv("CHARLY_RUNENV_TEST", "from-process")
	ctx := WithRunEnv(context.Background(), RunEnv{"CHARLY_RUNENV_TEST": "from-ctx"})
	if v, ok := RunEnvGet(ctx, "CHARLY_RUNENV_TEST"); !ok || v != "from-ctx" {
		t.Fatalf("RunEnvGet = (%q,%v), want (from-ctx,true)", v, ok)
	}
}

// TestRunEnvGet_FallsBackToProcessEnv proves the legacy single-invocation host CLI
// path is byte-identical to pre-RunEnv behavior: a ctx with no RunEnv (or a key
// absent from it) reads os.Getenv.
func TestRunEnvGet_FallsBackToProcessEnv(t *testing.T) {
	t.Setenv("CHARLY_RUNENV_TEST", "from-process")
	for _, ctx := range []context.Context{nil, context.Background(), WithRunEnv(context.Background(), RunEnv{"OTHER": "x"})} {
		if v, ok := RunEnvGet(ctx, "CHARLY_RUNENV_TEST"); !ok || v != "from-process" {
			t.Fatalf("ctx=%v RunEnvGet = (%q,%v), want (from-process,true)", ctx, v, ok)
		}
	}
	if v, ok := RunEnvGet(context.Background(), "CHARLY_RUNENV_MISSING"); ok || v != "" {
		t.Fatalf("missing key = (%q,%v), want (\"\",false)", v, ok)
	}
}

// TestWithRunEnv_EmptyIsNoOp proves a nil/empty env does not stamp a ctx value — so a
// caller that computes no overrides leaves the legacy process-env fallback intact.
func TestWithRunEnv_EmptyIsNoOp(t *testing.T) {
	ctx := context.Background()
	if got := WithRunEnv(ctx, nil); RunEnvFrom(got) != nil {
		t.Fatal("WithRunEnv(nil) stamped a RunEnv")
	}
	if got := WithRunEnv(ctx, RunEnv{}); RunEnvFrom(got) != nil {
		t.Fatal("WithRunEnv(empty) stamped a RunEnv")
	}
}

// TestDefaultDeployConfigPath_CtxOverrideWins proves the path resolver honors the
// ctx RunEnv override — the reason a roster's beds never share one overlay.
func TestDefaultDeployConfigPath_CtxOverrideWins(t *testing.T) {
	t.Setenv(DeployConfigEnv, "/process/charly.yml")
	ctx := WithRunEnv(context.Background(), RunEnv{DeployConfigEnv: "/bed/charly.yml"})
	got, err := DefaultDeployConfigPath(ctx)
	if err != nil {
		t.Fatalf("DefaultDeployConfigPath: %v", err)
	}
	if got != "/bed/charly.yml" {
		t.Fatalf("path = %q, want /bed/charly.yml", got)
	}
	// No ctx (legacy host CLI) still reads the process env.
	got, err = DefaultDeployConfigPath()
	if err != nil {
		t.Fatalf("DefaultDeployConfigPath(): %v", err)
	}
	if got != "/process/charly.yml" {
		t.Fatalf("path = %q, want /process/charly.yml", got)
	}
}

// TestDefaultDeployConfigPath_ConcurrentBedsDistinctOverlays is the decisive
// regression for the headline roster deadlock (RCA issue 1): N beds resolve their
// deploy-config path concurrently, and EVERY bed must get its OWN path — so two
// beds never contend on one overlay lock. With process-global os.Setenv (the
// retired mechanism) every bed read one shared value and they serialized/hung.
func TestDefaultDeployConfigPath_ConcurrentBedsDistinctOverlays(t *testing.T) {
	const n = 64
	type result struct {
		bed  int
		path string
		err  error
	}
	results := make(chan result, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			ctx := WithRunEnv(context.Background(), RunEnv{
				DeployConfigEnv: fmt.Sprintf("/tmp/bed-%02d/charly.yml", i),
			})
			p, err := DefaultDeployConfigPath(ctx)
			results <- result{bed: i, path: p, err: err}
		}(i)
	}
	seen := make(map[string]int, n)
	for i := 0; i < n; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("bed %d: %v", r.bed, r.err)
		}
		want := fmt.Sprintf("/tmp/bed-%02d/charly.yml", r.bed)
		if r.path != want {
			t.Fatalf("bed %d resolved %q, want %q (cross-bed bleed)", r.bed, r.path, want)
		}
		if other, dup := seen[r.path]; dup {
			t.Fatalf("beds %d and %d resolved the SAME overlay %q — they would contend on one lock", other, r.bed, r.path)
		}
		seen[r.path] = r.bed
	}
}
