## Core types

### `InstallPlan`

```go
type InstallPlan struct {
    DeployID       string       // per-deploy hash of image + add_layers
    Image          string
    Version        string       // layer/image CalVer
    Distro         string       // "fedora:43"
    Layer          string       // set for per-layer plans; "" for merged whole-image plans
    Steps          []InstallStep
    CandiesIncluded []string     // ordered topo-sorted layer names (for merged plans)
    AddCandies      []string     // refs added via charly.yml add_candy: (for provenance)
    BuilderImage   string       // selected builder for VenueContainerBuilder steps
    Meta           map[string]string
}
```

One plan per layer when the compiler runs on a single `Candy`. For whole-image deploys, `MergePlan(plans, image, addCandies)` merges per-layer plans while preserving layer boundaries for refcount bookkeeping. `DeployID` is a deterministic 16-hex-char sha256 prefix over `(image, layer_order, add_layers)` — same inputs → same ID, so re-deploys are stable.

### `InstallStep` interface

```go
type InstallStep interface {
    Kind() StepKind
    Scope() Scope
    Venue() Venue
    RequiresGate() Gate
    Reverse() []ReverseOp
}
```

All the builtin concrete step kinds (plus the open `external:<word>` family — F3's `externalStep`) implement this interface. `Reverse()` is called at install time (not teardown) so the ledger records the exact reversal ops tied to the specific artifacts created. (`ExternalPluginStep` is the exception that proves the rule: its `Reverse()` is static-nil — its reversal ops are DYNAMIC, recorded from the plugin's `OpExecute` reply at emit time, not from the step itself.)

### Enums

**`Scope`** — where the effect lands:
- `ScopeSystem` — `/etc`, `/usr`, `/var`; requires sudo on host; `USER root` in Containerfile.
- `ScopeUser` — `$HOME/.pixi`, `$HOME/.cargo`, `$HOME/.local`, user-scope systemd units.
- `ScopeUserProfile` — shell init surface (`~/.bashrc` / `~/.zshenv` / fish conf.d + `~/.config/opencharly/env.d/`).

**`Venue`** — where commands physically execute:
- `VenueHostNative` — host shell (plain or sudo-wrapped) or a plain `RUN` in Containerfile.
- `VenueContainerBuilder` — inside a builder container: `FROM <builder> AS stage` in Containerfile; `podman run <builder>` on host target.
- `VenueSkip` — recorded with reason but not executed (container-runtime-only fields on host target; `aur:` on non-Arch hosts).

**`Phase`** — three-phase template execution:
- `PhasePrepare` — repo config, key import, copr enable.
- `PhaseInstall` — the actual package-manager or builder invocation.
- `PhaseCleanup` — teardown (copr disable, cache wipe).

Each `SystemPackagesStep` carries one phase; `--allow-repo-changes` gating is a simple lookup on `step.Phase == PhasePrepare`.

**`Gate`** — opt-in flag name:
- `GateNone` (default, always enabled)
- `GateAllowRepoChanges`
- `GateAllowRootTasks`
- `GateWithServices`

Gates apply only to the host target. `EmitOpts.AssumeYes` enables all three. See `GateEnabled(gate, opts)` in `install_plan.go`.

**`StepKind`** — discriminator for concrete types:
- The CLOSED builtin set: `SystemPackages`, `Builder`, `Op`, `File`, `ServicePackaged`, `ServiceCustom`, `ShellHook`, `ShellSnippet`, `RepoChange`, `ApkInstall`, `LocalPkgInstall`, `Reboot`, `ExternalPlugin`.
- The OPEN external family (F3): `external:<word>` — a PLUGIN-contributed step kind served by a `class:step` provider. Not a fixed enum entry; carried OPAQUELY (see `externalStep` in the table below).

The Go-internal builtin vocabulary is the `AllStepKinds` slice in `provider_step.go` (all the builtin kinds — each round-trips through `stepToView`/`stepFromView`, the deploy view). `checkStepProviderBijection` asserts every kind is SERVED, split by how: a kind in `pluginEmitStepWords` (the plugin-served build-emit kinds — the PURE `File`/`ShellHook`/`ShellSnippet`/`ServicePackaged`/`ServiceCustom`/`RepoChange`/`ApkInstall` (C1.1) + the no-op-emit `Reboot` (C1.6, `Emits=false`) + the HOST-COUPLED `SystemPackages` (C1.2) + `Builder` (C1.3) + `LocalPkgInstall` (C1.4) + `Op` (C1.5)) resolves to a compiled-in `class:step` plugin declaring a `StepContract` (`candy/plugin-installstep`, NO in-proc `StepProvider`); the remaining kind (`ExternalPlugin`) resolves to its in-proc `StepProvider` (its `EmitOCI`). Add an in-proc kind → add it to `AllStepKinds` + a dedicated `StepProvider`; externalize a kind's build-emit → move it to `pluginEmitStepWords` + the plugin. An `external:<word>` kind is NOT in `AllStepKinds` and registers no per-word `StepProvider` — it is recognized by the `external:` prefix (`IsExternalStepKind`) and dispatched by the OPEN DEFAULT ARM in `kit.WalkPlans` + `RunHostStep`, so the closed bijection deliberately never sees it.

The IR carries no image-fetch step kind. Deploys (any target) emit
zero image-pull / image-build steps; test-bed image preflight is a
separate, check-time concern handled by `candy/plugin-check/preflight_images.go`
(the check-run preflight arm, K-wave 2 cone R4 — the former
`charly/check_image_preflight.go` is DELETED)
(the project rulebook "Deploy fetches NOTHING speculative" (`AGENTS.md`)).

## The step kinds (the builtin kinds + the open `external:<word>` family)

| Kind | What it carries | Venue default | Scope derivation |
|---|---|---|---|
| `SystemPackagesStep` | Format (rpm/deb/pac), Phase, Packages, Repos, Options, Copr, Modules, Exclude, Keys, CacheMounts | HostNative | Always system |
| `BuilderStep` | Builder (pixi/npm/cargo/aur), BuilderImage, CandyDir, Phase, Artifacts, RawStageContext | ContainerBuilder | aur→system, others→user |
| `OpStep` | Op (raw), CandyName, CandyDir, CtxPath, ResolvedUser | HostNative | From ResolvedUser (root or 0 → system; else user) |
| `FileStep` | Source, Dest, Mode, Owner, CandyName | HostNative | `pathIsSystemScoped(Dest)` |
| `ServicePackagedStep` | Unit, TargetScope, Enable, OverridesText, OverridesPath, CandyName, PriorEnabled | HostNative | TargetScope field |
| `ServiceCustomStep` | Name, UnitText, UnitPath, TargetScope, Enable, CandyName | HostNative | TargetScope field |
| `ShellHookStep` | CandyName, EnvVars, PathAdd, EnvFile | HostNative | Always user-profile |
| `ShellSnippetStep` | CandyName, Origin, Shell (bash/zsh/fish/sh), Snippet, PathAppend, Destination, Marker, UseDropin, Priority | HostNative | `pathIsSystemScoped(Destination)` (system for container drop-ins, user-profile for ~/.bashrc etc.) |
| `RepoChangeStep` | Format, File, Content, Checksum, CandyName | HostNative | Always system |
| `ApkInstallStep` | Packages (apk specs), CandyName, CandyDir | HostNative | Always system. Only `target: android` executes it; every other target records a skip. |
| `LocalPkgInstallStep` | PkgbuildRef, CandyName, CandyDir, ProjectDir, Format, LocalPkg (`*LocalPkgDef`) | HostNative | Always system. Compiled from the embedded `format.<fmt>.local_pkg:` block (the candy-body `localpkg:` map is REMOVED — nFPM cutover) at "step 2.5" (before tasks); `CompileLocalPkgStep` resolves the target distro's format FIRST (`DistroDef.LocalPkgFormat`), so `Format` + `LocalPkg` come from the `format.<fmt>.local_pkg:` block in the embedded `charly/charly.yml` — EVERY command rendered from config, no hardcoded build/install/glob literals. On a localpkg-capable deploy target (`target: local` / `target: vm`) the HOST runs the sdk's localpkg machinery (`sdk/deploykit/localpkg.go`): the PRODUCTION path (`!DevLocalPkg`) DOWNLOADS the published package from the per-distro repo (`LocalPkg.DownloadTemplate`, the glob derived from the URL via `localPkgGlobFromDownload`), while the DEV path (`--dev-local-pkg`) builds it via the `charly generate-packages` plugin (nFPM, `sdk/packagekit`) from the in-development binary — then installs it onto the target via `LocalPkg.InstallTemplate` — the format's AUTO-RESOLVING local-file install (`pacman -U` / `dnf install` / `apt-get install`), which pulls the package's repo deps automatically. There is NO dependency-closure builder. `LocalPkg.Probe` gates the install leg. At IMAGE build the install is **mode-switched by box type** (unified in `deploykit.RenderLocalPkgImageInstall`, shared by `sdk/deploykit WriteCandySteps` (the `charly box build`/`generate` image build, relocated in #67) + the pod-overlay build-emit (rendered DIRECTLY by `candy/plugin-installstep`'s `"oci-dispatch"` word against its own `deploykit.Generator` via the fetched `"resolved-project"` envelope since K5-Unit-6b — the former host `step-emit` seam `stepEmitLocalPkgInstall` is GONE) — R2): a PRODUCTION box DOWNLOADS the published release (`LocalPkg.DownloadTemplate`), while a DISPOSABLE check bed BUILDS the package via the plugin from LOCAL in-development source and COPYs it in — see "Check-vs-production charly toolchain" below. Skipped only on a distro with no localpkg-capable format (the layer's task fallback). Machinery: `sdk/deploykit/localpkg.go` (relocated from `charly/localpkg.go` in W3 — pure, no `*Config`/registry dependency); config: the embedded `charly/charly.yml` `format.<fmt>.local_pkg:` block. |
| `RebootStep` | CandyName | HostNative | Always system; `Reverse()` empty. Emitted last when a layer sets `reboot: true`. Its BUILD-emit is a plugin-served no-op (`pluginEmitStepWords[Reboot]="reboot"` → `candy/plugin-installstep`, `Emits=false` — an image build reboots nothing, C1.6; NO in-proc `StepProvider`, mirroring `ApkInstall`). Its DEPLOY leg is unchanged: only the external `vm` deploy reboots — its `candy/plugin-deploy-vm` walk drives the step host-side over `RunHostStep` → `rebootVenueAndWait`, where the host reboots the guest + waits for the deterministic boot_id change (a rebootable venue). OCI/pod/kubernetes skip; the external `local:` deploy skips + warns (never reboots the operator host). |
| `ExternalPluginStep` | Op (the desugared plugin-verb op — verb word + internal `plugin_input` + `RunAs`), CandyName, ResolvedUser, Distros | HostNative | From ResolvedUser (`opStepScope`: root/empty → system; else user). `Reverse()` static-nil — teardown ops are recorded DYNAMICALLY from the plugin's `OpExecute` reply. A plugin-verb `run:` step (authored `<word>: <input>`) whose verb is served by an EXTERNAL (out-of-process) `class:verb` plugin — see "ExternalPluginStep notes" below. |
| `deploykit.ExternalStep` (F3, kind `external:<word>`) | Word, ScopeV/VenueV/GateV (plugin-DECLARED), Payload (opaque `json.RawMessage`), CandyName, reverseOps (dynamic) | DECLARED (StepContract.venue) | DECLARED (StepContract.scope) — advisory, since the plugin self-execs `RunSystem`/`RunUser`. A PLUGIN-CONTRIBUTED step kind: a plugin-word `run:` step (authored `<word>: <input>`) whose word is a `class:step` provider declaring a `StepContract`. `compileActOp` lowers it (resolve(ClassStep) + the carrier's contract → `deploykit.ExternalStep` with the opaque desugared `plugin_input` as Payload) — **only when the word is NOT a verb (verb-first precedence)**: a plugin-word step whose word resolves as a `class:verb` is a verb act (falls through to `OpStep`), so a `class:step` word COLLIDING with a verb word (`file` = `verb:file` act + `step:file` build-emit, C1.1) resolves as the VERB, never a spurious `external:file`; `stepToView`/`stepFromView` round-trip it with the Scope/Venue/Gate AUTHORITATIVE (not recomputed); the host's OPEN DEFAULT ARM in `kit.WalkPlans` + `RunHostStep` dispatches it via the shared `invokeExternalStep` (`charly/plugin_executor_reverse.go`, S4/R2) → `Invoke(OpExecute)` over the SAME PLUGIN↔PLUGIN `InvokeProvider` leg `ExternalPluginStep` uses. `Reverse()` returns the dynamic ops recorded from the reply. THE carrier M2 reuses to externalize the builtin step kinds. The `StepContract` declaration rides `ProvidedCapability.step_contract` over Describe (`spec/proto`); example: `candy/plugin-example-stepkind`. **BUILD leg (F-STEP-EMIT):** `StepContract.Emits` (proto `step_contract.emits`) declares the step produces a build-context Containerfile FRAGMENT; the pod-overlay dispatch's external-step arm (`candy/plugin-installstep`'s `"oci-dispatch"` word, reached from the host's thin `charly/oci_step_emit.go` forwarder over the candy's `deploykit.OCITarget.EmitStepOp` → `HostBuild("step-emit","oci-emit-step")`) resolves the `class:step` provider by the trimmed word via `DescribeProvider`, and for `Emits=true` Invokes `OpEmit` via `InvokeProvider` (K5-A item 2, the plugin-side mirror of the SAME `invokeOpEmitFragment` shape `invokeOpEmitFragmentOpt` uses host-side) and splices the fragment; `Emits=false` → skip (a deploy-only external step, like apk on an image build). A HOST-COUPLED step (system-packages/builder/local-pkg-install/op) needs no host round-trip at all: `candy/plugin-installstep` fetches the `"resolved-project"` envelope once per project dir (K5-Unit-6b) and renders directly against its own `deploykit.Generator`. |

**Build-emit externalization (C1.1) — the seven PURE kinds.** `FileStep`, `ShellHookStep`,
`ShellSnippetStep`, `ServicePackagedStep`, `ServiceCustomStep`, `RepoChangeStep`, and
`ApkInstallStep` keep their typed structs + `Scope()`/`Venue()`/`Reverse()` methods (they still
compile from candy fields and project to the deploy view), but their pod-overlay BUILD-emit no
longer lives in an in-core overlay-walker `emit*` method — it moved to the compiled-in `class:step` plugin
`candy/plugin-installstep`. The candy's `deploykit.OCITarget` reaches the plugin over the
`HostBuild("step-emit","oci-emit-step")` seam (`dispatchOCIStep`, a thin host-side
forwarder); the plugin's OWN `"oci-dispatch"` word maps each kind to its serving word via
`deploykit.PluginEmitStepWords`, serializes the step VIEW (`StepToView`) as the `OpEmit` payload,
resolves the target via `DescribeProvider` + `InvokeProvider` (K5-A item 2), and splices
the plugin's fragment. The plugin decodes `spec.InstallStepView` and renders the SAME string the
former `emitFile`/`emitShellHook`/… produced (pure formatting — no host build engine). Their DEPLOY
leg is UNCHANGED (`kit.WalkPlans` `walkFile`/`walkShellHook`/… render from the same view). This is
build-emit-only externalization: full externalization (compiler → `externalStep`, deploy via
`OpExecute`) is the later M2 carrier above.

**Build-emit externalization (C1.2/C1.3/C1.4/C1.5) — the HOST-COUPLED `SystemPackagesStep` + `BuilderStep` + `LocalPkgInstallStep` + `OpStep`.** The
same build-emit-only pattern extends to the HOST-COUPLED kinds: each keeps its typed struct + methods +
native compile (`CompileSystemPackageSteps` / `compileBuilderSteps` / `CompileLocalPkgStep` / `compileActOp` for Op) and its UNCHANGED deploy leg
(host-engine via `RunHostStep` → `RenderHostPackageCommand` for SystemPackages, → `runVenueBuilderStep`
for Builder, → `deploykit.ExecLocalPkgInstall` for LocalPkgInstall; the act-OpStep `resolveProvisionScript` path for Op — the former `renderOpCommand` wrapper was a dead-code-radical-removal-batch deletion, test-only, zero production callers), but its pod-overlay BUILD-emit moved off the former in-core overlay walker into `candy/plugin-installstep`
(`pluginEmitStepWords[SystemPackages] = "system-packages"`, `pluginEmitStepWords[Builder] = "builder"`, `pluginEmitStepWords[LocalPkgInstall] = "local-pkg-install"`, `pluginEmitStepWords[Op] = "op"`).
UNLIKE the seven PURE kinds, SystemPackages/Builder/Op's render needs a build ENGINE — SystemPackages the `DistroDef`
format templates + `RenderTemplate` (C1.2); Builder the multi-stage `Generator.buildStageContext` +
`RenderTemplate` engine + the `BuilderConfig` registry + the box `ResolvedBox` (C1.3);
Op the RICHEST — `Generator.EmitTasks`, the full per-verb render pipeline (COPY from the layer scratch
stage, content-addressed inline-content staging, mkdir/link/setcap coalescing, the act-verb `case "plugin"`
seam) reading the scanned `Generator.candyByName` + the box `ResolvedBox` + `ImageBuildDir` + `ContextRelPrefix`
(C1.5). K5-Unit-6b eliminated the former host-callback resolution: the plugin's `"oci-dispatch"` word fetches the
`"resolved-project"` envelope ONCE per project dir and renders ALL FOUR host-coupled kinds DIRECTLY against its OWN
`deploykit.Generator` (built via the shared `deploykit.NewRenderGeneratorFromProject`) — there is NO call back to a
host `step-emit` renderer and NO `EmitReply` echo. LocalPkgInstall (C1.4) was already in this shape (W3): its render
`deploykit.RenderLocalPkgImageInstall` (→ the `charly generate-packages` plugin build + host-dir staging for a
dev bed, the release-download RUN for production) is a PURE `sdk/deploykit` function of its step argument — no `*Config`,
no live `*Candy` graph, no host callback — and it now renders the SAME way, against the plugin's own Generator,
no longer routing through a host `step-emit` seam for `buildEngineContext` threading. The former in-core per-word
renders (`stepEmitSystemPackages` / `stepEmitBuilder` / `stepEmitLocalPkgInstall` / `stepEmitOp` in
`step_emit_hostbuild.go`) are GONE; the SAME resolvers/pipeline the former in-proc overlay-walker build-emit used run
plugin-side (byte-equivalent): SystemPackages via `DistroCfg.FindFormat` → `phase.install.container`; Builder via
`buildStageContext` → `kit.BuilderResolve` (C10 — the SAME render the box-build path + the plugin's OpResolve use for the
externalized pixi/npm/aur multi-stage + cargo inline; a custom builder still via its vocabulary
`phases.install.container`); LocalPkgInstall via the SAME
`deploykit.RenderLocalPkgImageInstall` `sdk/deploykit WriteCandySteps` calls for the image build; Op via the SAME
`Generator.EmitTasks` `WriteCandySteps` (deploykit) calls for the box build, invoked from the plugin's own Generator
instance. The only host seam left is the thin `oci-emit-step` dispatch forwarder: the candy's `deploykit.OCITarget`
reaches the plugin's `"oci-dispatch"` word over `HostBuild("step-emit","oci-emit-step")` (`dispatchOCIStep`, a forwarder
NOT a render), so no `buildEngineContext` is threaded for a host-coupled render callback anymore.

**Build-emit externalization (C1.6) — the no-op-emit `RebootStep` — COMPLETES the 13-step set.** `RebootStep`
keeps its typed struct + `Scope()`/`Venue()`/`Reverse()` (empty) methods and its UNCHANGED deploy leg (the
host-side guest reboot over `RunHostStep` → `rebootVenueAndWait`, gated on the rebootable-VM-venue flag), but
its pod-overlay BUILD-emit moves off the last in-proc `rebootStepProvider.EmitOCI` (deleted) into
`candy/plugin-installstep` (`pluginEmitStepWords[Reboot] = "reboot"`). UNLIKE the HOST-COUPLED kinds it needs
NO host build engine and NO `stepEmitter`: a reboot has no meaning in an image build, so it is a NO-OP-emit
kind exactly like `ApkInstall` (C1.1) — it declares `Emits=false`, and the pod-overlay dispatch
(`candy/plugin-installstep`'s `"oci-dispatch"` word, K5-A item 2) skips its
`OpEmit` entirely (the plugin's `renderFragment` returns an empty fragment as a belt-and-suspenders
fallback). With C1.6 landed, the LAST host-coupled step kind's build-emit is externalized: EVERY builtin
InstallStep kind is now plugin-served — the compiler-emitted kinds via `candy/plugin-installstep`, and
`ExternalPlugin` via its own per-verb `class:step` plugin dispatch — leaving `ExternalPlugin` the
ONLY kind with an in-proc `StepProvider.EmitOCI`.

**`compileActOp` non-regression (the C1.1 verb-first fix).** C1.5 moves ONLY the `OpStep` BUILD-emit; `compileActOp`
(`install_build.go`) is UNCHANGED. A plugin-verb `run:` step (authored `<word>: <input>`) still lowers to a native
`OpStep` (verb-first precedence: an in-proc `ProvisionActor` / `command` verb wins over a colliding `step:` word —
the reason a `file:` step is the file VERB act, never an `external:file` step), and its DEPLOY leg still renders via
`resolveProvisionScript` (the former `renderOpCommand` wrapper around it was a dead-code-radical-removal-batch
deletion — test-only, zero production callers). `op` joins the compiler-emitted step-word set (never authorable —
no `op:` verb sugar exists), so it cannot
regress the verb-first path (`TestCompileActOp_VerbWordWinsOverCollidingStepWord`).

### Check-vs-production charly toolchain (the `--dev-local-pkg` distinction)

The charly toolchain installs the charly binary as a proper OS package on
every distro image. The BINARY SOURCE depends on the box type — a hard,
GENERIC distinction, NEVER mixed, decided in ONE place
(`deploykit.RenderLocalPkgImageInstall`, `sdk/deploykit/localpkg.go` — relocated from `charly/localpkg.go` in W3):

| Box type | charly binary source | How |
|---|---|---|
| **Disposable check box** (a `disposable: true` deploy) | latest **in-development** | the check-bed runner ALWAYS passes `charly box build --dev-local-pkg`, so the localpkg is BUILT via the `charly generate-packages` plugin (nFPM, `sdk/packagekit`) from the in-development binary + plugins and COPY'd into the image — a bed tests the charly code under development |
| **Production box** (any normal `charly box build`) | latest **published** | the default: the localpkg DOWNLOADS the published package from the per-distro repo (`LocalPkg.DownloadTemplate`, e.g. `https://opencharly.github.io/charly-arch/${ARCH}/charly-${ARCH}.pkg.tar.zst`) |

Both install via the SAME dep-resolving `InstallTemplate` (`pacman -U` / `dnf
install` / `apt-get install`) — only the package SOURCE differs, so the toolchain
is OS-tracked either way. The switch flows through `Generator.DevLocalPkg` ←
`BuildCmd.DevLocalPkg` (`--dev-local-pkg`); the check-bed runner sets it for EVERY
bed image build (`candy/plugin-check/bed_run.go`, driving `charly box build` over
the `HostBuild("cli")` seam), a production build never does. Generic
across all kinds + all localpkg candies. A dev build that cannot build via the
plugin **HARD-ERRORS** — it NEVER silently falls back to the release (R3), so a
bed can never accidentally test a stale published binary. Net: a disposable check
bed always exercises the in-development charly; a real box ships the released one.

**`ShellSnippetStep` notes:**
- Compiled by `CompileShellSnippetSteps` in `install_build.go` — applies the per-shell-wins-over-generic selection rule from `layer.Shell()`.
- `deploykit.OCITarget` emits a `RUN mkdir -p ... && cat > <dest> <<EOF` heredoc with a sha256-derived end-marker (anti-collision).
- The external `local:` (`candy/plugin-deploy-local`) and `vm:` (`candy/plugin-deploy-vm`) deploy walks — the SAME `kit.WalkPlans`, the vm one over the guest `SSHExecutor` — probe `command -v <shell>` once at the top of the walk; absent shells become VenueSkip-style no-ops with a logged reason. UseDropin=true → whole-file write; UseDropin=false → the kit walk's managed-block splice (`sdk/kit/profile.go`) against the existing rc file with a per-layer marker.
- Reverse: `ReverseOpRmFileSystem` / `ReverseOpRmFileUser` for drop-ins; `ReverseOpRemoveManaged` (with `Extra["marker"]=CandyName`) for managed-block append.
- Round-trip: `LabelShell` (`ai.opencharly.shell`) carries the merged set; `CollectShell` builds it at `charly box build` time, `ExtractMetadata` parses it at deploy time. The label's Deploy section is now permanently empty: the validation-correctness batch retired the deploy-scope `shell:` overlay AUTHORING FIELD itself (`#Deploy.shell`/`#DeployShellOverlay` deleted from `spec/schema/deploy.cue`, `charly migrate`'s `strip-deploy-shell-overlay` step migrates it away) — no longer merely "parses but doesn't overlay" (a transitional state this doc previously described), the field cannot be authored at all anymore.

**`ExternalPluginStep` notes** (`candy/plugin-installstep/oci_dispatch.go`'s `dispatchExternalPluginVerb` + `charly/plugin_executor_reverse.go`'s `invokeExternalStep` — the former `charly/plugin_step_external.go`/`externalPluginStepProvider` are DELETED, K-wave 2):

- **What it is.** The install-timeline IR node for a plugin-verb `run:` step (authored `<word>: <input>`) whose verb is served by an EXTERNAL (out-of-process) plugin — a `grpcProvider`, not a built-in `TypedStepProvider` (package/service) nor a `ProvisionActor` (the state-provision shell verbs) nor the `command` install verb. It is operator-authorized build/deploy-time plugin execution: the candy author opted in by composing the verb-step.
- **Routing (`compileActOp`, `install_build.go`).** A `plugin:` verb resolves through `providerRegistry.ResolveVerb`; if its provider is NOT a `TypedStepProvider` but DOES satisfy `executorInvoker` (an `InvokeWithExecutor` method, implemented SOLELY by the out-of-process `grpcProvider`), it routes to `ExternalPluginStep`. Every builtin `ProvisionActor` verb and `command` fall through to the generic `OpStep` path, unchanged. The `executorInvoker` capability is the precise discriminator — it mirrors the build-context `BuildEmitter` marker (`provider_verb.go`), placement-agnostic above the registry.
- **EmitOCI — the BUILD venue** (image build / pod-overlay Containerfile): `Invoke(OpEmit)` via the SHARED `invokeOpEmitFragmentOpt` seam (R2 — the SAME path `sdk/deploykit/tasks_render.go`'s `EmitTasks` `case "plugin"` takes for an external verb), splicing the plugin's Containerfile FRAGMENT verbatim. It cannot deploy-execute at build (no live venue); a deploy-only plugin (empty `OpEmit` fragment) fails loudly at `invokeOpEmitFragmentOpt`'s empty-fragment guard, never bakes nothing silently.
- **The DEPLOY venue — the shared `invokeExternalStep` dispatch** (the install runs ON the target, not into an image): there is NO `EmitLocal`/`EmitVM` on `StepProvider` (only `EmitOCI` remains — both deploy venues externalized with `target:local`/`target:vm`). When the external `local:`/`vm:` deploy walk reaches an `ExternalPluginStep` (a host-engine step), the host drives it over `RunHostStep` → `invokeExternalStep` (`charly/plugin_executor_reverse.go`, S4/R2 — the SAME shared dispatch `deploykit.ExternalStep` uses): `Invoke(OpExecute)` over the PLUGIN↔PLUGIN `InvokeProvider` leg (a nested reverse channel delegating to the SAME venue executor), so the plugin runs its deploy-context effect (RunSystem/RunUser) on the real venue it cannot hold across the process boundary, and RETURNS its teardown `ReverseOp`s. The host appends `reply.ReverseOps` to the `CandyRecord` in the HOST-side ledger (keyed by `computeDeployID`, like every external deploy — for a vm deploy too, NOT a guest-side ledger) — record-and-replay: only recorded ops are reversed at `charly deploy del`, reusing the SAME `spec.DeployReply` / `ReverseOp` wire the deploy-substrate dispatch (`plugin-fleet/candy/plugin-fleet/deploy_target.go`, R2) uses. `plugin_input` rides `op.Params` UNWRAPPED; a `spec.DeployVenue` rides `op.Env`. A `DryRun` short-circuits BEFORE the wire call (Invoke IS the apply).
- **`OpExecute` is the deploy-context counterpart of `OpEmit` (build).** Same verb-step, two legs: bake a Containerfile fragment at build (`OpEmit`), or execute the effect on a live `local:`/`vm:` target at deploy (`OpExecute`) — picked by venue, placement-agnostic.
- **No longer self-registers, and there is no provider to register.** `charly/plugin_step_external.go` was deleted with its `externalPluginStepProvider` and that kind's in-proc `EmitOCI` (`charly/plugin_dispatch_reverse.go:63-67`, K5-A item 2), leaving **zero live callers** of `EmitOCI`/`StepProvider` for this kind. The pod-overlay build-emit dispatch relocated into `candy/plugin-installstep`'s `oci-dispatch` word (`dispatchExternalPluginVerb`); the deploy-time side is `charly/plugin_executor_reverse.go`'s `invokeExternalStep`. There is no `registerDedicatedBuiltin` in any module.

Each step's `Reverse()` emits typed `ReverseOp` values. Adding a step kind means: (a) define the struct in `install_plan.go`, (b) decide its Scope/Venue/Gate/Reverse, (c) wire its BUILD-emit through a `class:step` plugin `OpEmit` referenced from `deploykit.PluginEmitStepWords` (`candy/plugin-installstep`) — a PURE kind renders the fragment directly from the step VIEW (the C1.1 pattern; a no-op-emit kind like `ApkInstall`/`Reboot` declares `Emits=false` and renders an empty fragment, so the pod-overlay dispatch's `dispatchClassStep` skips its `OpEmit`), a HOST-COUPLED kind (whose render needs project/box/candy structure) instead fetches the `"resolved-project"` envelope once per project dir and renders directly against its own `deploykit.Generator` (the C1.2/C1.3/C1.4/C1.5 pattern, `emitSystemPackages`/`emitBuilder`/`emitLocalPkgInstall`/`emitOp` in `candy/plugin-installstep/plugin.go` — no host round-trip beyond the ONE `InvokeProvider("build","project", OpResolve)` fetch of the resolved-project envelope, cached, K5-Unit-6b — the former `HostBuild("resolved-project")` seam is DELETED); a still-in-proc kind keeps an in-proc `StepProvider.EmitOCI` (only `ExternalPlugin` remains such after C1.6) — plus its DEPLOY leg in the shared `sdk/kit.WalkPlans` (and `RunHostStep` if it is a host-engine kind), (d) ensure the compiler in `install_build.go` emits it.

## The compiler — `BuildDeployPlan`

```go
func BuildDeployPlan(layer *Candy, img *ResolvedBox, hostCtx HostContext) (*InstallPlan, error)
```

Pure — no I/O, no side effects. Given the same inputs, produces the same plan. Called ONLY by the deploy command path (`deploy_add_cmd.go`), never by `charly box build`/`generate`:
- Once per layer during a pod deploy with `add_candy:` (the candy `plugin-deploy-pod` filters to `add_candy:`, `deploykit.OCITarget` walks the combined output for overlay synthesis).
- Once per layer during a VM or external deploy (local/kubernetes/android) — the target walks the combined output.

Pass `HostContext{Target: "host", Distro: ..., GlibcVersion: ...}` for host compilation; zero-value for the pod-overlay container compilation.

Step emission order (mirrors today's `WriteCandySteps` (deploykit)):
1. `ShellHookStep` for `env:` + `path_append:` (deterministic map ordering).
2. ONE `SystemPackagesStep` for the image's primary format — resolved via the most-specific-first distro CASCADE over `ResolvedBox.Distro` (e.g. `[ubuntu:24.04, ubuntu]`) plus the layer's top-level `package:` base: packages UNION across every matching per-distro tag section, while `repo`/`copr`/`option`/`exclude`/`module` resolve most-specific-wins. No per-distro section ever shares a mutable format section, so a deb-family repo (trixie vs noble) resolves deterministically. The cascade lives in **ONE shared function `ResolveCascadePackages` (`sdk/deploykit/install_build.go`)** called by BOTH the deploy compiler (`CompileSystemPackageSteps`) AND the image-build Containerfile emitter (`sdk/deploykit WriteCandySteps`) — there is exactly one package-resolution path, so a layer's packages are identical whether built into an image or applied at deploy. (Non-primary build formats like `aur` are a separate multi-stage builder concern, not a distro tag, and emit from their own format section.)
3. `OpStep`(s) in YAML order.
4. `BuilderStep`(s) for each matching multi-stage or inline builder.
5. `ServicePackagedStep` / `ServiceCustomStep` from the `service:` list — per-entry routing via `IsPackaged()` + `ServiceSchema.SupportsPackaged`.

`MergePlan([]*InstallPlan, image, addCandies)` composes per-layer plans into a single whole-image plan for target-level walking (sudo batching, single dry-run output).
