package proc

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/opencharly/spec/spec"
)

// RunCharlySubcommand shells out to the CURRENT binary with args, inheriting
// stdin/stdout/stderr. Relocated from charly core (run_subcommand.go) as a
// stdlib-only host-exec leaf: the host-side update/deploy orchestration
// (podUpdateCmd, the unified-target Update/Rebuild methods, per-kind R10
// sequences, `charly vm cycle`, deploy member bring-up/teardown) spawns child
// `charly <args…>` invocations through it, and resolving the build-under-test
// means an update loop (or a member fork) picks up the local build automatically.
//
// The child binary resolves via os.Executable() — an ABSOLUTE, chdir-immune
// path to the running process's binary — falling back to os.Args[0] only on
// os.Executable()'s error (a genuinely rare OS-level failure, e.g. the exe was
// deleted mid-run on some platforms). os.Args[0] ALONE is wrong here: it is
// whatever string invoked the process — a bare `charly` (PATH-resolved,
// historically the only thing operators ran, so the bug stayed masked) or a
// RELATIVE path like `./bin/charly`. A relative os.Args[0] resolves against the
// process's CURRENT working directory at fork time, not the directory it had
// when os.Args[0] was captured — so a caller that os.Chdir()s (e.g. `charly -C
// box/fedora …`) before this var forks a child (e.g. deploy-member bring-up's
// `charly config <member>`) sends the child hunting for
// box/fedora/bin/charly, which doesn't exist (ENOENT). os.Executable() has no
// such relative-path hazard — it is resolved once, absolutely, at the OS level.
//
// A package var (not a plain func) so tests can stub the child-process boundary
// (e.g. record the image-build / vm-cp-box calls a deploy makes without actually
// spawning the binary).
//
// This is the LEGACY no-ctx form: it inherits the parent's process environment
// verbatim (byte-identical to the pre-RunEnv behavior) and is correct for an
// operator-path fork, where no per-invocation overrides exist. A caller that
// CARRIES per-invocation overrides on ctx (a concurrent in-process bed roster
// threads CHARLY_DEPLOY_CONFIG / CHARLY_REPO_OVERRIDE / CHARLY_PREEMPT_LEASE as
// spec.RunEnv) must use RunCharlySubcommandCtx so the child receives them as
// explicit data — otherwise the child falls back to the real per-host overlay
// and a bed member writes the operator's state (measured live).
var RunCharlySubcommand = func(args ...string) error {
	return RunCharlySubcommandCtx(context.Background(), args...)
}

// RunCharlySubcommandCtx is RunCharlySubcommand with the invocation's per-call
// environment merged over the inherited one. The env comes from the ctx RunEnv
// (spec.RunEnvFrom — the plan §4.2 explicit-data carrier); a ctx with no RunEnv
// yields a nil child env, so os/exec inherits the parent environment unchanged
// (byte-identical to RunCharlySubcommand). A package var so tests can stub the
// child-process boundary.
var RunCharlySubcommandCtx = func(ctx context.Context, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	return runCharlySubcommandCtx(ctx, exe, args)
}

// runCharlySubcommandCtx is the testable exec body: it runs exe with args,
// inheriting stdin/stdout/stderr and merging the ctx RunEnv over the parent env.
// Extracted so a test can exercise the cmd.Env wiring against a real child (a
// shell that prints the env) without os.Executable being a charly binary.
func runCharlySubcommandCtx(ctx context.Context, exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = mergedChildEnv(spec.RunEnvFrom(ctx))
	return cmd.Run()
}

// mergedChildEnv returns the child environment: the parent's own environment
// with extra merged OVER it (a per-call key REPLACES the inherited value). A
// nil/empty extra returns nil, so os/exec inherits the parent env unchanged.
func mergedChildEnv(extra spec.RunEnv) []string {
	if len(extra) == 0 {
		return nil
	}
	base := os.Environ()
	idx := make(map[string]int, len(base))
	for i, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok {
			idx[k] = i
		}
	}
	for k, v := range extra {
		entry := k + "=" + v
		if i, ok := idx[k]; ok {
			base[i] = entry
		} else {
			base = append(base, entry)
		}
	}
	return base
}
