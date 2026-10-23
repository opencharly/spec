## Op selectors at build vs deploy

A provider's `Invoke` carries an `op.Op` selector (`charly/provider.go`). Three drive THIS subsystem (the build/deploy IR), placement-agnostically (in-proc for a builtin, go-plugin gRPC for an external); a fourth, `OpRun`, is the runtime/CLI selector OUTSIDE the install-plan IR (listed here for selector completeness):

- **`OpEmit`** is INVOKED at IMAGE-BUILD time — a `run:` plugin verb / plugin step returns a `spec.EmitReply.Fragment` (with a `spec.BuildEnv` descriptor in `op.Env`) that the generator splices verbatim into the Containerfile (egress-validated). This is a BUILD-mode concern, dispatched from `EmitTasks` (`sdk/deploykit/tasks_render.go`), NOT the IR/`deploykit.OCITarget` — EXCEPT the pod-overlay render, where `ExternalPluginStep.EmitOCI`'s `class:verb` dispatch AND the authored `external:<word>` `class:step` step (declaring `StepContract.Emits=true`, F-STEP-EMIT) both route through `candy/plugin-installstep`'s `"oci-dispatch"` word (K5-A item 2), which Invokes `OpEmit` on the resolved target via `InvokeProvider` — the plugin-side mirror of the SAME `invokeOpEmitFragment` shape (R2) `invokeOpEmitFragmentOpt` uses host-side. See `/charly-build:generate` + `/charly-internals:generate-source`.
- **`OpResolve`** is ALSO INVOKED at IMAGE-BUILD time — the BUILDER leg, serving BOTH the four DETECTION-builders (pixi/npm/aur/cargo, C10) and the `external_builder:`-selected out-of-tree builders. A `ClassBuilder` provider returns a `spec.BuilderResolveReply` (`{Stage, CopyArtifacts, CopyBinary, InlineFragment}`, with a `spec.BuildEnv` in `op.Env` + a `spec.BuilderResolveInput` render context in `op.Params`): `Stage` (a `FROM <ref> AS <name>` block) splices PRE-main-FROM, `CopyArtifacts`+`CopyBinary` (`COPY --from=<stage> …`) POST-main-FROM, and an INLINE builder's (cargo) `InlineFragment` splices in-candy. The detection builders are dispatched from `emitBuilderStages` / `emitBuilderArtifacts` (the host computes the full render context; the stage template lives in the plugins' `sdk/kit.BuilderResolve`); the `external_builder:` ones from `emitExternalBuilderStages` / `emitExternalBuilderArtifacts` — both share the `resolveBuilderStage` Invoke helper. The multi-stage counterpart of the verb/step `OpEmit` leg. See `/charly-build:generate` + `/charly-internals:generate-source`.
- **`OpExecute`** drives EXTERNAL deploy-context execution over the executor reverse channel — the ACTUAL substrate provider's `OpExecute` (reached by `candy/plugin-fleet`'s `handleDeployApply`/`handleLifecycleSimple`/… via its own `sdk.Executor.InvokeProvider` — S1, S3b), AND an `ExternalPluginStep` (or `deploykit.ExternalStep`) on a `local:`/`vm:` deploy walk, which the host executes via the shared `invokeExternalStep` dispatch (`charly/plugin_executor_reverse.go`, S4/R2 — `Invoke(OpExecute)` over the SAME PLUGIN↔PLUGIN `InvokeProvider` leg, driven over `RunHostStep` as the walk reaches the step) — both decoding the `spec.DeployReply` teardown record (above). `OpExecute` is the deploy-context counterpart of the build-context `OpEmit`: the SAME external verb-step bakes a fragment at build (`OpEmit`) and executes its effect on a live target at deploy (`OpExecute`), picked by venue.
- **`OpRun`** is the RUNTIME / CLI selector, OUTSIDE the install-plan IR (it neither builds an image nor applies a deploy plan): it runs a check verb / live-container probe (`provider_checkenv.go` `invokeVerbProvider`), AND it dispatches an EXTERNAL COMMAND plugin's `charly <word>` subcommand — `provider_command_external.go` `dispatchExternalCommand` forwards the pass-through CLI tokens as `op.Params = {"args":[…]}` (marshalled directly, no `plugin_input` envelope) on an `Invoke(OpRun)` to the lazy-connected out-of-process command provider. Owned by `/charly-internals:plugin` (the command class) + `/charly-check:check` (the verb probes).

## `StepBatch` batching

`InstallPlan.StepsByVenue()` partitions `Steps` into contiguous same-`(Scope, Venue)` runs. The `local:`/`vm:` deploy walk uses this to emit one shell heredoc per batch:

| Batch | Emission form |
|---|---|
| `{ScopeSystem, VenueHostNative}` | `sudo bash <<'CHARLY_ROOT' … CHARLY_ROOT` |
| `{ScopeUser, VenueHostNative}` | `bash <<'CHARLY_USER' … CHARLY_USER` |
| `{_, VenueContainerBuilder}` | `podman run <builder> bash -s < script` |
| `{_, VenueSkip}` | Logged, no exec |

`deploykit.OCITarget` doesn't batch — it emits each step in order as Containerfile directives; adjacent same-`USER` steps collapse naturally via the existing `USER` switching logic in `EmitTasks`.

## Deferred home resolution (`{{.Home}}` + `InstallPlan.ResolveHome`)

Home-bearing step fields carry the deferred `HomeToken` (`{{.Home}}`) rather
than a home expanded at compile time. `compileShellHookStep` always emits the
token; `CompileShellSnippetSteps` emits it for deploy targets (`hostCtx.Target`
== `host`/`vm`) and keeps `img.Home` for the container build. Each
`EmitTarget` calls `plan.ResolveHome(home)` once at emit with the home of the
**actual** destination:

| Target | Home used |
|---|---|
| `deploykit.OCITarget` (pod-overlay image build) | `img.Home` (the image's runtime home — the candy reads it from the envelope) |
| external `local:` deploy | host home (`ShellExecutor.ResolveHome`) |
| external `vm:` deploy | GUEST home (`SSHExecutor.ResolveHome`) |

`ResolveHome` substitutes the token in `ShellHookStep` (EnvVars + PathAdd),
`ShellSnippetStep` (Snippet + Destination + PathAppend), and `FileStep.Dest`;
it deliberately skips `OpStep` command/content bodies (`~`/`$HOME` there
shell-expand at runtime on the destination as the deploy user) and
`BuilderStep` (home resolved separately by `deploykit.RenderBuilderScript`,
`sdk/deploykit/localpkg.go`). It is
idempotent. Baking `img.Home` at compile time was the VM `$HOME` bug: the
synthetic plan's Home was the host operator's, so a guest deploy wrote env.d
pointing at `/home/<operator>` and user-scope installs (npm -g, cargo) landed
in a root-owned path the guest user couldn't write. See
`/charly-internals:vm-deploy-target` "Guest-home resolution".

## `ReverseOp` catalogue

See `/charly-local:local-deploy` for the user-facing reverse-op table. The Go-level source of truth is `ReverseOpKind` in `spec/spec/deploy_consts.go` (the IR's step kinds; `spec/spec/install_plan.go` — the former `charly/install_plan.go` is DELETED, K-wave 2); each step's `Reverse()` method emits ops tagged with kind + targets + scope. Execution lives in `sdk/kit/reverse_ops.go` (moved from `charly/reverse_ops.go`) — one handler per kind, all routed through `runReverseOps(ops, executor)` in LIFO order.

Adding a new reverse kind requires:
1. Add the `ReverseOpKind` constant in `install_plan.go`.
2. Emit it from the appropriate step's `Reverse()` method.
3. Add a handler function in `reverse_ops.go` and register it in `runReverseOp`'s dispatch switch.

## `EmitOpts` cross-cutting flags

```go
type EmitOpts struct {
    DryRun               bool
    FormatJSON           bool
    AllowRepoChanges     bool
    AllowRootTasks       bool
    WithServices         bool
    SkipIncompatible     bool
    AssumeYes            bool
    Verify               bool
    Pull                 bool
    BuilderImageOverride string
}
```

CLI flags on `DeployAddCmd` / `DeployDelCmd` populate this struct; each target reads what it needs. `AssumeYes` enables all three opt-in gates (via `GateEnabled`).

## Testing

- `install_plan_test.go` — 13 unit tests over step-kind derivations (scope/venue/gate/reverse).
- `install_build_test.go` — 8 integration tests that load real `candy/` via `ScanAllCandyWithConfig`; `testHostContextWithDistro` helper.
- `install_build.go:200` comment — canonical fixture docs.

When you add a step kind, add:
1. A scope/venue/gate/reverse unit test in `install_plan_test.go`.
2. An integration test in `install_build_test.go` exercising the compiler path.
3. Target-specific tests in `build_target_oci_test.go`, the shared out-of-process walk in `sdk/kit/walk_test.go`, and the host-engine channel in `plugin_executor_hoststep_test.go`.

