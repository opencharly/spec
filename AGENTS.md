# AGENTS.md — opencharly/spec

The OpenCharly **wire/IR contract module** (`github.com/opencharly/spec`): the
single-source CUE schema (`schema/`), the generated config/wire/InstallPlan-IR
types (`spec/`), the gRPC plugin transport (`proto/`), the CUE protocol model
(`protocol/`), and the small mechanism-free helper packages that ride along. It
is the dedicated contract `charly` core AND every plugin import — extracted from
`opencharly/sdk` so core depends only on the contract, never on a mechanism kit.

Canonical files:

- `charly.yml` — the project manifest and the `kind: task` maintenance surface
  (`bootstrap-cue`, `bootstrap-protoc`, `cue-gen`, `wire-gen`/`proto-gen`).
- `schema/*.cue` — the single source of truth for the `charly.yml` ingress
  schema and the wire shapes (embedded as `schema.FS`).
- `spec/cue_types_gen.go`, `spec/vocab_gen.go` — GENERATED Go (never hand-edit).
- `protocol/schema/*.cue` + `proto/plugin.proto` + `proto/*.pb.go` — the CUE
  protocol model and its GENERATED gRPC stubs.
- `internal/schemagen`, `internal/wiregen` — the generation pipelines (not
  importable).
- `README.md` — user overview only; never agent guidance.

## Load these skills first (R0)

- `/charly-internals:go` — the SDD operationalization: the schema→generated-code
  pipeline map, the schema-change recipe, and the generation-coverage state.
- `/charly-internals:plugin` — the per-plugin CUE-schema contract and the
  provider model the wire types serve.

## Build / validate / test

- `charly task cue-gen` — regenerate every CUE-owned artifact from
  `schema/*.cue` (`spec/cue_types_gen.go`, `spec/vocab_gen.go`, and the proto
  stubs). Reproducibility-gated: a clean regen is a no-op, `TestGenReproducible`
  enforces it.
- `charly task wire-gen` (alias `proto-gen`) — regenerate `proto/` from
  `protocol/schema/*.cue`.
- `GOWORK=off go test ./...` / `go build ./...` — the module's Go gates. CI
  (`.github/workflows/ci.yml`) runs build, test, gofmt and golangci-lint.
- The merge gate is the **org-wide** `charly/pr-validator` (required check
  `validate / validate`, defined in `opencharly/.github`); this repo carries no
  per-repo candy gate.

## Modify this repo

- Define authored configuration and wire shapes in CUE before code. Edit
  `schema/*.cue`, then regenerate — never hand-transcribe a schema-shaped wire
  struct. A clean regeneration must be a no-op.
- There is no schema-version stamp to bump. The `#SchemaVersion` CalVer, the
  `charly.yml` `version:` field and the equality gate were DELETED
  (`CHANGELOG/0.2026270.938.md`); compatibility is enforced by unifying the
  authored document against the CLOSED CUE schema, where an unknown or removed
  field is a hard `field not allowed` error. A change that alters an AUTHORED
  wire key makes `charly migrate` the consuming repo's problem and is declared
  as such; a pure `@go()` annotation change (a Go identifier, or a pointer /
  tri-state spelling such as `@go(,type=*bool)`) preserves the wire key and
  needs no version machinery at all.
- Go-module tags follow the `v0.<YYYYDDD>.<HHMM>` scheme.

## Landing

- PR-only. Every change lands through a pull request; the org-required
  `charly/pr-validator` validates the diff and body and arms native auto-merge on
  PASS. Direct pushes to `main` are blocked.
- History lives in `CHANGELOG/` (written by `tag-on-merge` at merge time); the PR
  body IS the changelog.
- The authoritative rulebook is the umbrella `AGENTS.md` in
  `opencharly/opencharly` and `charly/AGENTS.md` in the charly repo. Do not
  restate its rules here.
