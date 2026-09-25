package spec

import (
	"context"
	"os"
)

// runenv.go — the PER-INVOCATION run environment, carried on context.Context.
//
// A check-bed run needs three process-scoped values (the per-bed deploy-config
// path, the repo override, and the preempt-lease ownership marker). Historically
// these were set with os.Setenv so a bed's forked `charly` children inherited
// them. That is CORRECT only when one bed runs per process. A roster that runs
// many beds as goroutines in ONE process shares os.Setenv globally, so bed A's
// value is read by bed B (and by bed B's children) — cross-bed contamination;
// two concurrent beds then contend on each other's deploy-config lock and hang.
//
// The fix (plan §4.2, F8 placement-invariance): thread the values as EXPLICIT
// DATA per invocation. A bed's context carries its RunEnv; every path resolver
// an invocation reaches reads the ctx value FIRST, falling back to os.Getenv only
// for the legacy single-invocation host callers that never set one. Children get
// the same values via spec.CliRequest.Env (never via inherited process env).

// RunEnv is the per-invocation environment override map. A nil/absent RunEnv
// means "no override" — every reader falls through to os.Getenv, byte-identical
// to the pre-RunEnv behavior for the host CLI.
type RunEnv map[string]string

type runEnvCtxKey struct{}

// WithRunEnv returns a context carrying env as this invocation's run environment.
// A nil/empty env is a no-op (returns ctx unchanged).
func WithRunEnv(ctx context.Context, env RunEnv) context.Context {
	if len(env) == 0 {
		return ctx
	}
	return context.WithValue(ctx, runEnvCtxKey{}, env)
}

// RunEnvFrom returns the invocation's run environment, or nil when none is set.
func RunEnvFrom(ctx context.Context) RunEnv {
	if ctx == nil {
		return nil
	}
	if env, ok := ctx.Value(runEnvCtxKey{}).(RunEnv); ok {
		return env
	}
	return nil
}

// RunEnvGet resolves key for an invocation: the ctx RunEnv value wins; absent
// that, os.Getenv (the legacy single-invocation behavior); a miss returns ("", false).
func RunEnvGet(ctx context.Context, key string) (string, bool) {
	if env := RunEnvFrom(ctx); env != nil {
		if v, ok := env[key]; ok {
			return v, true
		}
	}
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}
	return "", false
}
