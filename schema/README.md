# spec/schema — the single-source CUE schema

Every `charly.yml` shape (box / candy / vm / kubernetes / deploy / …) is defined
ONCE here, in the one CUE package `schema` (the module
`github.com/opencharly/spec` is declared in `cue.mod/module.cue`). These files
are the single source of truth for these consumers:

1. **Runtime validation (ingress).** The schema travels WITH this module: its
   source as `schema.FS` (package `github.com/opencharly/spec/schema`) and the
   module that owns it as `specmodule.FS` (package `github.com/opencharly/spec`).
   charly core (`charly/cue_schema.go` `sharedCueSchema`) composes it into ONE
   compiled `cue.Value` and validates every loaded entity against its `#<Kind>`
   def. The composition is the consumer's business; the declaration it composes
   with is this module's, so it is exported here for any consumer that composes
   the module by import. See `/charly-build:validate`.
2. **Generated Go param types + vocabulary (`spec`).** `charly task cue-gen`
   turns these same files into the committed `spec/*_gen.go` — so the Go
   structs the loader decodes into, and the kind/verb/method word lists the CLI
   dispatches on, can never drift from what the schema validates.
3. **The package itself.** `cue eval ./schema` loads the directory as the
   `schema` package and evaluates the same value the concatenation compiles;
   that is the form a consumer's own CUE module imports (a plugin schema
   importing `github.com/opencharly/spec/schema` instead of holding a copy).

> Every file carries exactly ONE package-level clause — `package schema` — and
> `ConcatSchema` strips it, so the body it returns stays byte-identical to the
> pre-package one. (CUE excludes a file whose name starts with `_` from a
> package load, which is why the shared definitions live in `00_common.cue`: the
> `00_` sorts first, keeping the concatenation's order — and so the generated
> Go — byte-identical.) The `package spec` clause + the file-level `@go(spec)`
> attribute that the generator reads are injected by the gen pipeline
> (`internal/schemagen -mode=params` → `schemaparams`), NEVER written into the source.

## The package form

`schema/` is one CUE package in the module `github.com/opencharly/spec` (declared by
`cue.mod/module.cue`), so it loads as a package rather than as a pile of package-less
files; a consumer's own CUE module imports `github.com/opencharly/spec/schema`. The
repo's supported command stays `charly task cue-gen`, which compiles these same files
through the pinned toolchain it provisions and gates the result with
`TestGenReproducible`.

A consumer that composes the module itself — one that embeds the schema's source instead
of reading a checkout — needs BOTH halves, because the declaration sits at this module's
root, outside `schema/`'s `//go:embed *.cue` tree: the package source is `schema.FS`, and
the module that owns it is `specmodule.FS` (package `github.com/opencharly/spec`).
Composing with only the files would evaluate them under the consumer's toolchain default
instead of the pinned version above.

`cue.mod/module.cue` pins language version `v0.16.1` — the version the embedded
`cuelang.org/go` library is pinned to in `go.mod`, and the ONE declaration both the
in-process generator and every composing consumer evaluate under. It is not
incidental: the schema's `close()` + embedded-definition idiom (`#Step`'s arms
embedding the closed `#Op`) is legal through v0.17 and rejected by v0.18's semantics
with `field not allowed`. Raising the language version is the §10.1 pattern rewrite
(TODO 2.16), not a bump.

## Regenerating `spec` — `charly task cue-gen`

In THIS repository:

```sh
charly task cue-gen
```

The task is `cue-gen` in this repo's `charly.yml` (the `kind: task` surface). It
depends on `wire-gen` (which in turn depends on `bootstrap-protoc`):

1. **Concatenates** `schema/*.cue` via `internal/schemagen -mode=params`
   (the SAME `schemaconcat.ConcatSchema` contract as the runtime
   `sharedCueSchema` — one concatenation contract; the per-file `package
   schema` clause is stripped), headed `package spec` + `@go(spec)`.
2. **Composes and generates in-process** (`schemaparams`): the concatenation is
   loaded as ONE CUE module instance through `schemacompose` (the same offline
   resolution a plugin's served schema uses), and the Go param structs are
   generated from the CUE Go API → `spec/cue_types_gen.go`. No `cue` CLI is
   provisioned or executed.
3. **`internal/schemagen -mode=vocab`** → `spec/vocab_gen.go`
   (`KindWords`, `DocDirectives`, `StepKeywords`, `ContextWords`, `OpFields`,
   `OpVerbs`, `AuthoringVerbs`, … — all read straight from the compiled schema
   via the cue API).
4. **`gofmt`** the committed generated files.

`wire-gen` (chained first) additionally renders `protocol/schema/*.cue` into
`proto/plugin.proto` and compiles the Go gRPC stubs.

Each plugin's OWN `params/cue_types_gen.go` is generated from its own
`candy/plugin-*/schema/*.cue` in the plugin's OWN repository, via the SAME
`schemaparams` pipeline (R3) — including a schema that imports
`github.com/opencharly/spec/schema`, which resolves through the same offline
compose and emits the type through the Go package that owns it
(`github.com/opencharly/spec/spec`). The former superproject
`taskfiles/Cue.yml` wrapper — which chained this repo's `cue:gen` and then looped
over each plugin — was removed with the candy de-submodule work (charly#452,
merged 2026-08-29); plugin param generation now lives with the plugin.

The spec task rename (`cue:gen` → `cue-gen`) has no cross-repo consumer: the
superproject wrapper that once invoked `spec/Taskfile.yml`'s `cue:gen` no longer
exists on charly's `origin/main` (verified: `git -C charly ls-tree -r
origin/main --name-only | grep -c taskfiles/Cue.yml` → `0`).

These runs are **reproducible**: two consecutive `charly task cue-gen`
invocations produce no diff, and `TestGenReproducible` (in `spec/`) fails CI if
the committed `*_gen.go` differs from a fresh regeneration. **Never hand-edit
the `*_gen.go` files** — change the `*.cue` source and regenerate.

## The `@go(...)` annotations

The params generator is lossy: every CUE disjunction degrades to `any` /
`map[string]any` / an empty struct (Go cannot express a disjunction), and field
names get a naive `leadingCap + underscores-preserved` mapping. Two annotation
classes in the `*.cue` source fix this — both are **inert for runtime validation**
(CUE attributes never participate in unification). Schema comments that name
`gengotypes` describe this mapping: the generator is a faithful in-process port of
`cue exp gengotypes` v0.16.1 (`schemaparams/gotypes.go`), so each statement still holds:

- **Field renames** — `@go(<GoName>)` makes the generated Go field name match the
  hand-written charly param struct (`http` → `HTTP`, `cap_add` → `CapAdd`,
  `kernel-param` → `KernelParam`, …). The wire (JSON/YAML) key is preserved.
- **Union-type suppression + redirect** — the handful of disjunction defs whose
  faithful Go shape charly relies on are annotated `@go(-)` (suppressing the lossy
  generated type) and hand-written in `spec/union_types.go` (the SAME
  package, so the generated structs reference them by name). The current set is
  the source itself: grep `schema/*.cue` for `@go(-)` and read the matching
  hand-written types in `spec/union_types.go` — an enumerated copy here would
  drift. A scalar-containing
  disjunction is suppressed with a trailing value attribute — `(…) @go(-)` — so
  `{} & string` can never collapse the validation; an all-struct disjunction takes
  a bare trailing `@go(-)`.

## Egress schemas

The egress schemas (the CUE charly validates the config it WRITES against) do NOT
live here. They live in the superproject's compiled-in
`candy/plugin-egress/egress-schemas/` (+ `vendor/cloud_config.cue`), with
`candy/plugin-deploy/egress.go` as a thin shim — see `/charly-internals:egress`. So
`schema/*.cue` here is purely the INGRESS schema; only the `node.cue`
node-disjunction grammar defs appear as (harmless) generated types in
`cue_types_gen.go` (excluded from param-gen by `excludeParamGen`).
