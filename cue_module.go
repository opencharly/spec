// Package specmodule shares THIS Go module's own CUE module, so a Go consumer can
// compose `github.com/opencharly/spec/schema` as a CUE MODULE INSTANCE instead of
// concatenating its files into one package-less compilation unit.
//
// The schema's source travels in the sibling `schema` package (`schema.FS`). What
// does NOT travel there is the module that owns it: `cue.mod/module.cue` sits at
// this module's root, outside `schema`'s `//go:embed *.cue` tree, so it is
// unreachable from `schema` — and it carries the one fact a consumer cannot derive
// from the files, the PINNED `language.version` the schema's `close()` +
// embedded-definition idiom (`schema/00_common.cue`'s `#Op` arms) needs. CUE
// evaluates under the version the module declares, so a consumer that guessed it
// from its own toolchain would be applying semantics the schema never agreed to.
// Restating the declaration is therefore not an option either: the module path and
// its language version are declarations only this module may make.
//
// A consumer overlays this FS and `schema.FS` at one virtual module root — this
// one at `<root>/cue.mod/module.cue`, the schema's files at `<root>/schema/` — and
// loads `./schema` from there.
package specmodule

import "embed"

// FS holds this module's CUE module declaration at `cue.mod/module.cue`, the path
// CUE's loader requires relative to a module root.
//
//go:embed cue.mod/module.cue
var FS embed.FS
