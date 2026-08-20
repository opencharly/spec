# opencharly/spec

The OpenCharly **wire/IR contract module**: the CUE schema (`schema/`), the generated config /
wire / InstallPlan-IR types (`spec/`), the gRPC plugin transport (`proto/`), and the small
mechanism-free helper packages that ride along. The dedicated contract that `charly` core
**and every plugin** import — extracted from `opencharly/sdk` as part of the #55 import-purity
program so core depends only on the contract, never on a mechanism kit.

## Package inventory

| Package | Role |
|---|---|
| `schema/` | The single-source CUE schema (`*.cue`, package-less) — embedded as `schema.FS` |
| `spec/` | The generated + hand-written config/wire/IR types (`cue_types_gen.go`, `vocab_gen.go`, `version_gen.go`, `union_types.go`, …) — the module's core |
| `proto/` | The generated gRPC plugin transport (`plugin.proto` + `*.pb.go`) |
| `protocol/` | The CUE protocol model that `task wire:gen` renders into `proto/` |
| `schemaconcat/` | The one concatenation contract (`ConcatSchema`) shared by runtime validation and the gen pipeline |
| `capability/` | The SDK-facing authoring surface (`ProvidedCapability`, `CLISubcommand`, …) that `BuildCapabilities` marshals into the proto wire forms |
| `calver/` | CalVer parse/compare + the schema-version gates (`ParseCalVer`, `CompareCalVer`, `LatestSchemaCalVer`) — sliced out of `spec/` |
| `merge/` | Pure `UnifiedFile` map/struct merges (`MergeUnified`, `MergePluginKindsMap`) — sliced out of `spec/` |
| `loader/` | The loader discover/scan shape (`LoaderDiscover`, `ScanSpec`) — sliced out of `spec/` |
| `matchers/` | The goss-style matcher evaluation engine (`MatchAll`) over the `spec.Matcher` value type — sliced out of `spec/` |
| `poll/` | The unified poll/readiness subsystem (`PollUntil`, `PollCondition`, `ReadinessProvider`, poll bounds) — sliced out of `spec/` |
| `fleet/` | Fleet-node operations over the `spec.FleetNode` value type (`ResolveNodePath`, `ClassifyNodeTarget`, `MergeFleetNode`, …) — sliced out of `spec/` |
| `http/` | HTTP check helpers over the `spec.CheckHTTP*` wire types — sliced out of `spec/` |
| `shellquote/` | The canonical POSIX single-quoter (`ShellQuote`) — sliced out of `spec/` |
| `exec/`, `proc/`, `transport/`, `sshx/`, `exitcode/` | Executor / process / transport / ssh / exit-code helpers |
| `refs/`, `lock/`, `ops/`, `phase/`, `container/`, `checkhost/`, `checkstep/` | Ref resolution, lock, op, phase, container, check-host/step helpers |
| `climodel/`, `clireflect/`, `hostenv/`, `report/`, `testkit/` | CLI model/reflection, host-env, report, test-kit helpers |
| `internal/` | `schemagen` (the gen pipeline) + `wiregen` (the proto renderer) — not importable |

## Generate

- **`task cue:gen`** — spec types from `schema/*.cue` (reproducibility-gated: a clean regen is a
  no-op, `TestGenReproducible` enforces it).
- **`task wire:gen`** — proto from `protocol/schema/*.cue`.

## Go-module tags

The current rule is `v0.<YYYYDDD>.<HHMM>` (semver-legal CalVer, leading zeros stripped) — the
`sdk`-style scheme this module shares. The first 15 tags (2026-08-25 → 2026-08-30) predate the
rule and were stamped `v2026.225.1508`-style (no `0.` after `v`); they are history, not the
current format. New tags follow the `v0.<YYYYDDD>.<HHMM>` rule.
