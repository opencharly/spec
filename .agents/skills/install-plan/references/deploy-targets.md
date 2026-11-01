## The `EmitTarget` interface

```go
type EmitTarget interface {
    Name() string                 // "oci" | "pod" | "host" | "vm:<name>"
    Emit(plans []*InstallPlan, opts EmitOpts) error
}
```

NO in-proc targets implement the bare `EmitTarget` (Name + Emit) interface — the former in-proc overlay walker + the pod overlay target were DELETED in P11c (the pod overlay render now lives in the candy `plugin-deploy-pod`, via `deploykit.OCITarget`), so there are no deploy targets dispatched by `ResolveTarget`. The deploy LIFECYCLE is the separate `UnifiedDeployTarget` interface (Add/Del/Update/Start/Stop/Status/Logs/Shell/Rebuild — `Test` DELETED, #55 W3 B3 remainder: zero real callers anywhere in the tree), and its sole implementer is the thin, data-only `pluginDeployTarget` (`charly/unified_targets.go`, S3b — see "`pluginDeployTarget` + `candy/plugin-fleet`" below): ALL FIVE substrates — `local`, `vm`, `pod`, `kubernetes`, `android` — route through it over `candy/plugin-fleet`'s `Invoke(OpDeployDispatch)` and, from there, the executor reverse channel to the ACTUAL substrate's own out-of-process plugin:

### `deploykit.OCITarget` (`sdk/deploykit/oci_target.go`) — the pod-overlay walker (P11c relocation)
The kind-blind Containerfile walker MOVED out of `charly/build_target_oci.go` (P11c) into `sdk/deploykit`; the candy `plugin-deploy-pod` constructs it + renders via `deploykit.NewRenderGeneratorFromProject`. Emits Containerfile text. Consumes `phases.install.container` from the embedded `charly/charly.yml` build vocabulary (the single source for build-mode package installation). All compiler-emitted step kinds' build-emit is plugin-served: the plugin-served build-emit kinds (`deploykit.PluginEmitStepWords` — the PURE C1.1 kinds + the no-op-emit `reboot` (C1.6) + the HOST-COUPLED `system-packages` (C1.2), `builder` (C1.3), `local-pkg-install` (C1.4), `op` (C1.5)) route through the host's thin `oci-emit-step` forwarding seam (`charly/oci_step_emit.go`'s `dispatchOCIStep`, reached by the candy's `deploykit.OCITarget.EmitStepOp` over `HostBuild("step-emit","oci-emit-step")`) to `candy/plugin-installstep`'s `"oci-dispatch"` word, which itself resolves the serving `class:step` provider via `DescribeProvider`/`InvokeProvider` and Invokes its `OpEmit` (K5-A item 2) — their former in-core overlay-walker `emit*` methods are gone; the payload is the step VIEW (`StepToView`), and a legitimately-empty render is tolerated (`allowEmpty`; a no-op-emit `apk-install`/`reboot` declares `Emits=false` and is skipped entirely). The HOST-COUPLED kinds (`system-packages`/`builder`/`local-pkg-install`/`op`) no longer call back a host `step-emit` renderer at all (K5-Unit-6b): `candy/plugin-installstep` fetches the `"resolved-project"` envelope once per project dir and renders them DIRECTLY against its own `deploykit.Generator` (built via the shared `deploykit.NewRenderGeneratorFromProject`) — `op`'s render still delegates to the SAME `Generator.EmitTasks` (the per-verb emitters `EmitCopy`/`EmitWrite`/`EmitCmd`/`EmitMkdirBatch`/...) the box build uses, just invoked from the plugin's own Generator instance rather than a host round-trip. Only `ExternalPlugin` keeps an in-proc `StepProvider.EmitOCI` at the Go-object level (dispatched to its `class:verb` provider via `InvokeProvider` from the plugin's `"oci-dispatch"` word) — the C1.1–C1.6 externalization is COMPLETE, every InstallStep kind now plugin-served.

Used by: pod deploys with `add_candy:` (overlay Containerfile synthesis) — the ONLY `deploykit.OCITarget` construction site (the candy `plugin-deploy-pod`'s `buildOverlay`). `charly box build` / `charly box generate` do NOT use `deploykit.OCITarget`; build-mode Containerfile emission is the separate `WriteCandySteps` → `EmitTasks` generator in `sdk/deploykit` (see the Overview "Build mode is a SEPARATE path").

### `pod` is EXTERNAL (`deploy:pod`, candy/plugin-deploy-pod) — the plugin walks NOTHING; the overlay builds HOST-SIDE
There is no in-proc pod deploy target. `pod:` resolves to `pluginDeployTarget` (below), served out-of-process by `candy/plugin-deploy-pod`'s `deploy:pod` provider. UNLIKE local/vm, pod does NOT walk the IR on a venue — pod bakes its install steps INTO the image at build time. Its registered `candy/plugin-deploy-pod` lifecycle (M4 + P11c) builds the overlay container image HOST-SIDE in `PrepareVenue`: the candy calls `HostBuild("overlay")` → the core prep+resolve M-seam (`charly/build_overlay.go` `hostBuildOverlay` — the pod-substrate sibling of the `buildengine-prep` box-build seam), which reconstructs the `*Generator`, resolves the base ref + distro + init + metadata, stages remote candy copies, projects a `*spec.ResolvedProject`, serializes the live plans, caches the `buildEngineContext`, and returns the `OverlayBuildReply` envelope; the candy then constructs a `deploykit.Generator` (via the shared `deploykit.NewRenderGeneratorFromProject`), renders the overlay Containerfile IN ITS OWN code (`deploykit.OCITarget` walker + the per-step `HostBuild("step-emit","oci-emit-step")` → `dispatchOCIStep` → `candy/plugin-installstep`'s `"oci-dispatch"` — the former in-core render DISSOLVED into the candy by P11c, and the dispatch DECISION itself further relocated there by K5-A item 2), and runs `podman build` + the deploy-name alias tag via the served executor. The overlay is synthesized for `add_candy:` (deterministic tag from `(base-image, sorted-layer-set)`, removed on `charly deploy del` unless `--keep-image`), or the base is tagged as the deploy-name alias when there is no `add_candy:`. The serializable `spec.OverlayBuildRequest` carries the scalars while the live plans + nested-venue `ParentExec`/`ParentNode` ride the ctx (a live executor cannot cross the `[]byte` payload). It returns a host-local `ShellExecutor`, so the generic `pluginDeployTarget`/`candy/plugin-fleet` apply dispatch is a clean no-op (the plugin's `Invoke` is a thin acknowledgment) and `recordVenueLedger` no-ops (a pod carries no venue-side ledger — its candies live in the image). The hook also owns the container lifecycle (`charly config`/`start`/`stop`/`status`/`logs`/`shell`; `Rebuild` = `box build`+`check box`+`deploy add`+`stop`+`config`+`start` — the `charly update` R10 fresh-rebuild gate; `PostTeardown` = `charly remove` + drop the `<name>-overlay` images). The bed runner drives a pod bed through the DEFAULT pod path (build → config → start → check-live → `charly remove`) exactly as the in-proc pod did — only `deploy add`'s overlay build internally routes through this hook now.

### `local:` is EXTERNAL (`deploy:local`, candy/plugin-deploy-local) — consumes the IR over the reverse channel
There is no in-proc local deploy target. `local:` resolves to `pluginDeployTarget` (below), served out-of-process by `candy/plugin-deploy-local`'s `deploy:local` provider (F1). UNLIKE kubernetes, the local substrate DOES consume the InstallPlan IR: the plugin walks the plan via the shared out-of-process walk (`sdk/kit.WalkPlans`), groups contiguous same-`(Scope, Venue)` steps via `StepsByVenue()`, emits one heredoc per batch, and writes service units (packaged + custom), env.d files, and managed blocks (the host records the returned teardown ops to the ledger via `install_ledger.go`). The plugin owns the walk ORDERING but cannot execute the host-engine step kinds itself — `BuilderStep`, `LocalPkgInstallStep`, `SystemPackagesStep` (host renders via `DistroConfig`), act-verb `OpStep` (host registry `resolveProvisionScript`), and `ExternalPluginStep` (nested plugin dispatch) are driven on the HOST via the `RunHostStep` reverse-channel RPC (the host owns the engine/registry). The host executor is `ShellExecutor` for `host:local`/absent and `SSHExecutor` for `host:<user@machine>`, picked by `rootExecutorForDeployNode` (`sdk/deploykit/deploy_chain.go`). See `/charly-local:local-deploy` for the user-facing surface and `/charly-internals:local-infra` for supporting files (hostdistro, ledger, reverse_ops, shell_profile, builder_run, `sdk/deploykit/compile_service_steps.go` (service_render, relocated), `candy/plugin-fleet/deploy_ref.go` (deploy_ref, relocated)).

### `vm` is EXTERNAL (`deploy:vm`, candy/plugin-deploy-vm) — consumes the IR over the reverse channel, INSIDE the guest
There is no in-proc VM deploy target. `vm:` resolves to `pluginDeployTarget` (below), served out-of-process by `candy/plugin-deploy-vm`'s `deploy:vm` provider — the vm-substrate sibling of `candy/plugin-deploy-local`. UNLIKE kubernetes, the vm substrate DOES consume the InstallPlan IR: the plugin walks the plan via the SAME shared `sdk/kit.WalkPlans` the local deploy uses. The difference is purely the executor's TRANSPORT — the executor the reverse channel serves for a vm deploy is the **guest `SSHExecutor`**, so the same walk runs INSIDE the guest (shell bodies exec via `ssh guest 'sudo bash -s'`). Plugin-renderable steps (Op/File/ShellHook + the env.d managed-block finalizer/ShellSnippet/Service*/RepoChange) the plugin executes itself via the reverse legs; host-engine steps (`Builder`/`LocalPkgInstall`/`SystemPackages`/act-`Op`/`ExternalPlugin`) AND the `RebootStep` it drives on the HOST via `RunHostStep` (so builders run on the host's podman + artifacts scp into the guest, and the host reboots the guest).

The VM venue lifecycle — boot the domain, build the guest `SSHExecutor` (`kit.WaitForSSH` → `kit.WaitForCloudInit` → `kit.EnsureCharlyInGuest`), return the `VmDeployState` patch, deploy nested `target: pod` children as in-guest quadlets, and teardown — is IMPLEMENTED IN THE PLUGIN `plugin-deploy-vm/candy/plugin-deploy-vm/lifecycle.go` (`Lifecycle:true`) over `sdk/kit` + `HostBuild("cli")` + the served guest executor. The plugin resolves its OWN `spec.LifecyclePrepareInput` end-to-end by self-loading the project directly (K-wave W3a A3-phase-2: `sdk/loaderkit.ResolveVmEntityViaExecutor` — the former host-side `lifecyclePrepareHook` DATA-seam AND the "deploy-entity-resolve" HostBuild seam it later used are BOTH GONE — FINAL/K5 unit 6a + K-wave W3a A3-phase-2). Core keeps NO residual cleanup for the vm substrate: the last survivor, the F12 `vmAttachResolver` (`charly/vm_lifecycle_preresolve.go`), was DELETED at K-wave 2 cone CONTESTED — the plugin derives the attach script from the raw wire cmd (`vmAttach` over `lifecycleParams.Cmd`), and `charly/unified_targets.go`'s Attach threads it for hookless lifecycle substrates. The vm `lifecyclePostTeardownHook` moved plugin-side too (`plugin-deploy-vm/candy/plugin-deploy-vm/lifecycle.go`'s `vmPostTeardown`). The lifecycle Ops are reached through `pluginDeployTarget` → `candy/plugin-fleet`'s `Invoke(OpDeployDispatch)` → the plugin's own `sdk.Executor.InvokeProvider` call into the vm substrate provider (S3b — the generic dispatch every substrate now shares, replacing the former dedicated `grpcSubstrateLifecycle` proxy), which also persists the returned `VmDeployState` via `saveDeployState`. The teardown ledger is keyed HOST-SIDE by `computeDeployID(name)` (like every external deploy); the recorded `ReverseOps` replay over the guest `SSHExecutor` (an `sshReverseRunner`), so teardown runs in the guest.

Used by: `charly deploy add vm:<name> <ref>` / `charly deploy del vm:<name>`. See `/charly-internals:vm-deploy-target` for the full flow, the plugin-implemented venue lifecycle, `VmDeployState` persistence, and SSH-key idempotency.

### kubernetes is EXTERNAL (`deploy:kubernetes`, candy/plugin-kube) — NOT an IR-consuming deploy target
There is no in-proc kubernetes deploy target. `target: kubernetes` resolves to `pluginDeployTarget` (below), served out-of-process by candy/plugin-kube's `deploy:kubernetes` provider (F1, beside its `kube:` verb). The Kustomize GENERATION is the COMPILED-IN `candy/plugin-k8sgen` (M13, `verb:k8sgen`/`OpEmit`) — split from the heavy external plugin-kube because the generator has no client-go dep and must resolve in the project-less `from-box` path; the package-main `Capabilities` is NOT crossed (only its 3 scalars Port/UID/GID, alongside `spec.Deploy`/`spec.Kubernetes`, in a `spec.KubernetesGenInput`). `charly/k8s_generate.go` + the former "k8s-generate-kustomize" HostBuild seam it served (`host_build_k8s_generate.go`) are BOTH DELETED (K5-A item 6): the WHOLE generation + validation step now runs ENTIRELY plugin-side, in `candy/plugin-kube/materialize.go`'s `materializeKustomize` — it InvokeProviders `verb:k8sgen`/`OpEmit` for the manifest docs and `verb:egress`/`OpValidate` (`#K8sObject`/`#Kustomization`) to validate each one, both peer-to-peer, no host round trip for either leg. The PLUGIN-side `deploy:kubernetes` preresolver (`candy/plugin-kube/preresolve.go`, F6/FINAL-K5-unit-6a) + `charly deploy from-box` (`candy/plugin-fleet/deploy_from_box.go`) both call `materializeKustomize` directly, which writes the `base/`+`overlays/` tree under `.opencharly/k8s/<name>/` and shipping the overlay path in `DeployVenue.Substrate` (`spec.KubernetesDeployVenue`). The plugin then runs `kubectl --context <ctx> apply -k` (the LIVE cluster I/O it owns) and returns the `kubectl delete -k` + tree-removal teardown op the host records. kubernetes does NOT consume the InstallPlan IR (a kubernetes deploy compiles no primary image plan). Cluster-specific choices come from a `kind: kubernetes` cluster template (the `kubernetes:` entity), resolved by the plugin's own self-load (`sdk/loaderkit.ResolveKubernetesEntityViaExecutor`, K-wave W3a A3-phase-2), not the InstallPlan. See `/charly-kubernetes:kubernetes` + `/charly-internals:plugin`.

### `pluginDeployTarget` (`charly/unified_targets.go`) + `plugin-fleet/candy/plugin-fleet/deploy_target.go` (S3b)

The `UnifiedDeployTarget`/`LifecycleTarget` adapter for ALL FIVE substrates. Unit-6 S3b moved the
orchestration bulk of the former dedicated `externalDeployTarget` (`charly/deploy_target_external.go`,
709 lines) + `grpcSubstrateLifecycle` (`charly/substrate_lifecycle_grpc.go`, 374 lines) — both
DELETED, along with `charly/deploy_preresolve.go`'s `wireDeployPreresolver` and
`charly/deploy_substrate_lifecycle.go`'s `substrateLifecycle` interface — into `candy/plugin-fleet`,
behind ONE generic selector: `sdk.OpDeployDispatch`. What is left core-resident is a THIN,
DATA-ONLY proxy:

- **`pluginDeployTarget`** (`charly/unified_targets.go`) holds ONLY plain data (name/word/
  hasLifecycle/hasPreresolve/node) plus a live venue executor — never a core-private `*grpcProvider`
  (constructed only at plugin-CONNECT time, a clause-M mechanism that cannot live in a plugin). Every
  `UnifiedDeployTarget`/`LifecycleTarget` method (`Add`/`Update`/`Del`/`Start`/`Stop`/`Status`/
  `Logs`/`Shell`/`Attach`/`Rebuild`) marshals a `spec.DeployTargetDispatchRequest{Op: "add"|"update"|
  "del"|…}` and calls its own `dispatch()`, which threads the current venue (`t.venueJSON`, reused
  across calls once the first "add" dispatch reports one back) and calls `dispatchDeployTarget`.
- **`dispatchDeployTarget`** (`charly/deploy_target_dispatch.go`) threads a live executor onto the
  ctx via the SAME "compiled-in in-proc reverse channel" pattern `arbiterInvoke` uses (the former
  `dispatchEphemeralOp` is DELETED — only a comment survives; no broker hop needed —
  `command:deploy` is COMPILED-IN today), then
  `Invoke`s `command:deploy`'s `sdk.OpDeployDispatch` with the marshalled request. Core never touches
  the substrate's `*grpcProvider` directly once this call returns.
- **`runDeployDispatch`** (`plugin-fleet/candy/plugin-fleet/deploy_target.go`) is the plugin-side handler: it
  recovers the threaded executor via `sdk.ExecutorForInvoke`, decodes the `spec.DeployTargetDispatchRequest`,
  and switches on `req.Op` to `handleDeployApply`/`handleDeployDel`/`handleLifecycleSimple`/
  `handleDeployStatus`/`handleDeployExec`/`handleDeployRebuild` — each of which marshals the
  deployment's `InstallPlan`s (as `spec.InstallPlanView`) + a `spec.DeployVenue` descriptor and
  reaches the ACTUAL substrate provider (candy/plugin-deploy-pod/-vm/-local, candy/plugin-kube,
  candy/plugin-adb) via its OWN `sdk.Executor.InvokeProvider` (S1) — placement-agnostic, whether
  that substrate is compiled-in or out-of-process. It decodes the structured `spec.DeployReply`
  (`{reverse_ops, record}`) and writes `ReverseOps` + provenance into the ledger via the SAME
  `install_ledger.go` path a built-in Add uses. **Before that ledger-persist, `recordDeploy` fills
  each `ReverseOpPackageRemove.UninstallCmd` from the deploy's `DistroConfig`** (now threaded as a
  plain marshalled field, not the core-only `buildEngineContext` wrapper) — the host-only
  `uninstall_template` render the out-of-process plugin cannot do itself (the aur builder's
  `kit.BuilderReverse` records the op with an EMPTY `UninstallCmd`, deferring to this render). Both
  the `local` AND `vm` substrates route through `Add → apply → recordDeploy`, so their aur-builder
  teardown resolves the `pacman -Rs …` command at `charly deploy del` instead of erroring on an empty
  command.
- **Test** (the interface method that used to run deploy-scope checks HOST-SIDE via
  `runUnifiedTargetChecks`) is DELETED (#55 W3 B3 remainder): it had zero real callers anywhere in
  the tree — `charly check live` reaches `candy/plugin-check` directly (`live_gather.go`'s
  `pluginCheckRunLive`), never through this adapter. The `target: local --verify` deploy-scope
  check pass (a DIFFERENT, still-live path) runs plugin-side too now — see
  `candy/plugin-fleet/verify_local.go`.
- **Update** re-dispatches with fresh plans — an idempotent re-Add (the candy's ledger `ReverseOps`
  are REPLACED, not appended).
- **Del** replays the RECORDED `ReverseOps` from the ledger (no plugin call) via the shared
  `teardownHostDeploy` — the record-and-replay invariant: only recorded ops are reversed, never
  recomputed.

**Nested-child venue threading — `applyParentExecOverride` (FIX ROUND, S3b follow-up, R10
bed-found regression).** A nested external deploy child with NO lifecycle hook of its own (a
`local:`/`android:`/`kubernetes:` node placed by TREE POSITION under a `vm:`/`pod:` parent) must apply
INSIDE the parent's already-prepared venue, never the operator host — mirroring the pre-move
`externalDeployTarget.apply`'s `else if opts.ParentExec != nil { t.exec = opts.ParentExec }` swap
exactly (the DELETED `charly/deploy_target_external.go:262`). The bug this fix closes: for a
nested child, `resolveRootExecutor` (`plugin-fleet/candy/plugin-fleet/deploy_target.go`) would silently fall
back to `deploykit.RootExecutorForDeployNode(req.Node)` — which, for a child carrying no `host:`
field of its own, defaults to the operator's host `ShellExecutor` — so every plain-vm nested
child's plan/step walk ran on the OPERATOR'S HOST instead of the guest venue. The fix:

- `pluginDeployTarget.applyParentExecOverride(opts)` (`charly/unified_targets.go`) is a NO-OP when
  `t.hasLifecycle` (a lifecycle substrate composes its OWN nested venue INSIDE its own
  `PrepareVenue`) or `opts.ParentExec == nil`. Otherwise it mutates `t.exec` to the live
  `opts.ParentExec` (so every subsequent reverse leg this dispatch call drives —
  `RunSystem`/`RunUser`/`RunHostStep`/… — runs against the PARENT's venue) AND flattens that same
  live executor into a `spec.VenueDescriptor` via `kit.DescriptorFromExecutor` — because a live Go
  interface value cannot itself cross the `[]byte` wire into the plugin's decoded
  `spec.DeployTargetDispatchRequest`. `pluginDeployTarget.Add` threads the result as the dispatch
  request's `VenueJSON`.
- `resolveRootExecutor` (`plugin-fleet/candy/plugin-fleet/deploy_target.go`) now checks `req.VenueJSON` FIRST:
  non-empty → decode + `kit.VenueFromDescriptor` re-materializes the IDENTICAL parent venue;
  empty → the original `deploykit.RootExecutorForDeployNode(req.Node)` fallback (correct for a
  TOP-LEVEL hookless deploy, which has no ancestor venue to inherit).
- The ordering invariant — `t.exec` is ALWAYS mutated together with the returned `venue_json`,
  never one without the other — is enforced by keeping `applyParentExecOverride` as its own
  directly-unit-tested method (`unified_targets_test.go`), not inlined at the one call site.

This closes exactly the gap the R10 bed roster (7/7 beds) exercises for a nested-child deploy;
see `spec/exec/venue_descriptor.go` above for the promoted `DescriptorFromExecutor`/
`VenueFromDescriptor` pair both directions now share.

**What stays core-resident BY DESIGN, wrapping the dispatch rather than living inside it** (Unit-6
design Q1–Q4, verified against the actual call graph, not assumed):
- **The arbiter bracket is fully PLUGIN-SIDE now** (K-wave 2 cone R2 bank E — the
  `charly/host_build_arbiter_bracket.go` F10 host-builders are DELETED): `candy/plugin-fleet/
  deploy_target.go`'s `handleLifecycleSimple` wires the acquire/release as
  `InvokeProvider("verb","arbiter",OpRun)` calls (arbiterBracketAcquire/arbiterBracketRelease) into
  the pure, directly-testable `runLifecycleBracket`, which brackets only when `HasPlan` and a
  non-nil `Node` and op is Start or Stop — acquire BEFORE dispatch, release on the failure path for
  Start, release AFTER dispatch unconditionally for Stop. `CHARLY_PREEMPT_LEASE` is process-ENV
  state (`os.Setenv`/`os.Getenv`); the outer-orchestrator GUARD lives in
  `candy/plugin-preempt`'s `invokeArbiter` (the arbiter is the one dispatch every acquire/release
  passes through, and plugin-fleet + plugin-preempt are both compiled-in, sharing charly's
  process — so a nested `charly` subprocess still inherits the lease and skips re-acquiring).
- **The pod Start/Stop/Attach/Logs plan-hook table read** (`pod_lifecycle_dispatch.go`, unmoved) — a
  pure ctx-opts marshal with zero core-only dependency of its own; the plan resolution the pre-move
  `grpcSubstrateLifecycle` ran as a SEPARATE pre-dispatch call now resolves INSIDE the plugin dispatch
  itself (Unit-6 design Q3), with the host-side plan-hook lookup running BEFORE the arbiter bracket is
  entered.
- **Secret injection, artifact retrieval, and `--verify`** — ALL plugin-side now, inside
  `handleDeployApply` itself (`candy/plugin-fleet/secrets_artifacts.go`'s `injectCandySecrets`/
  `retrieveArtifactsAndDispatchRegisters`; `verify_local.go`'s `verifyLocalDeployScope`, #55 W3
  B3). `charly/unified_targets.go`'s `pluginDeployTarget.Add` no longer wraps any of these — it
  only dispatches and returns.

The ledger key is `computeDeployID(name, nil, nil)` — derived from the deploy name alone (so the
host-venue `Kind()=="host"` never collides with another host-venue deploy on the ledger scan). A
bed/deploy that uses an external deploy SUBSTRATE word is recognized at config-PARSE time by the
byte-gated, additive declaration pre-scan (`plugin_prescan.go`) before the provider connects; `charly
check live` / `charly check <verb>` route an external deploy host-side via the shared
`checkLocalTarget` classifier (`check_venue.go`) — the host-venue path the externalized `local:`
substrate itself takes (R2). The wire types (`InstallPlanView`, `DeployVenue`, `DeployReply`,
`ReverseOp`, `ReverseOpPluginScript`, `DeployTargetDispatchRequest`, `DeployTargetDispatchReply`) are
CUE-sourced at `spec/schema/deploy.cue` / `spec/schema/seam.cue`, generated into `spec/spec/cue_types_gen.go`
— SDK-importable so an out-of-tree deploy plugin constructs the same structs (R2).

