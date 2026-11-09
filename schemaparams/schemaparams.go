// Package schemaparams is THE schema→Go params pipeline (R3).
//
// One function runs the whole thing — concatenate a schema directory's `*.cue` files
// under a `package <pkg>` + `@go(<pkg>)` header, compose them as ONE CUE module
// instance through `schemacompose`, generate the Go structs in-process from the CUE Go
// API, then normalize the struct tags — so that the three places that need it cannot
// drift:
//
//   - spec's own generator (its `cue-gen` task) produces `spec/cue_types_gen.go`;
//   - the `charly`-side verb (`charly candy params`) produces a plugin candy's
//     `params/cue_types_gen.go`, and is a THIN wrapper over this;
//   - a plugin repo's CI can run it too, because this module is `go install`-able
//     (its tags are `v0.<YYYYDDD>.<HHMM>`) where the charly module's nested CalVer
//     tags are not — so a CI job gets the canonical pipeline without a charly
//     binary and without keeping its own copy of the recipe.
//
// The steps' contracts stay where they are, and this package only sequences them:
// `schemaconcat` (the concatenation), `schemacompose` (the one module-aware compose,
// with its offline resolution of the contract module), `schemaretag` (the tag
// normalization), and the in-process generator (`gotypes.go`).
//
// Composition and generation share ONE resolution path (opencharly/spec#271, Direction
// 1). A plugin's schema may carry `import "github.com/opencharly/spec/schema"`; the
// compose resolves it against the contract module the Go binary already links, with no
// published CUE module, no module cache and no `cue` CLI, and the generator emits the
// definition through the Go package that actually owns the shapes (`gohome.go`). That is
// what a projected params file needs to compile: `cue exp gengotypes` could do neither,
// because the CLI resolves modules only through an OCI registry and its generator lives
// in cuelang.org/go's INTERNAL tree.
package schemaparams

import (
	"fmt"
	"os"

	"github.com/opencharly/spec/schemacompose"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/schemaretag"
)

// Generate returns the Go file the schema directory produces.
func Generate(schemaDir, pkg string) ([]byte, error) {
	return GenerateExcluding(schemaDir, pkg, nil)
}

// GenerateExcluding is Generate with a file filter: a schema whose disjunction
// wrappers (spec's `node.cue`) are not param structs passes an `exclude` that names
// them. nil includes every `*.cue` file, which is what a plugin candy wants.
func GenerateExcluding(schemaDir, pkg string, exclude func(name string) bool) ([]byte, error) {
	body, files, err := schemaconcat.ConcatSchema(os.DirFS(schemaDir), ".", exclude)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", schemaDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no *.cue files in %s", schemaDir)
	}
	// The header is part of the contract: `package <pkg>` is re-established by the
	// compose, and the file-level @go(<pkg>) attribute is what the generator reads to
	// name the Go package.
	inst, err := schemacompose.ComposeText(nil, "@go("+pkg+")\n\n"+body, pkg)
	if err != nil {
		return nil, fmt.Errorf("composing %s as package %s: %w", schemaDir, pkg, err)
	}
	gen, err := generateGo(inst)
	if err != nil {
		return nil, fmt.Errorf("generating Go for %s: %w", schemaDir, err)
	}
	return schemaretag.Normalize(gen), nil
}
