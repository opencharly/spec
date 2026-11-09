package schemaparams

import (
	"sort"
	"strings"
)

// goHome names the Go package that owns a CUE package's generated shapes.
type goHome struct {
	ImportPath string
	Package    string
}

// goHomes is THE relation between a CUE package a schema may import and the Go package
// that owns its shapes. It cannot be derived: CUE import paths and Go import paths are two
// namespaces, and a schema's CUE package is the SOURCE of shapes whose generated Go lives in
// a different package. spec's CUE package `github.com/opencharly/spec/schema` is the source;
// its generated params (`spec/cue_types_gen.go`, package spec) live in
// `github.com/opencharly/spec/spec`. Emitting the CUE path as a Go import names a package
// that exports only FS, so the referenced type is undefined (opencharly/spec#298, F2).
//
// ONE entry today: spec's schema is the only CUE package the compose serves offline (a
// plugin's module can import nothing else — schemacompose's registry refuses every other
// path), so it is the only package a projection can reference across an import. A package
// that gains a Go home is an edit HERE, and the projection's test compiles a generated file
// against it.
var goHomes = map[string]goHome{
	"github.com/opencharly/spec/schema": {
		ImportPath: "github.com/opencharly/spec/spec",
		Package:    "spec",
	},
}

// registeredGoHomes lists the relations, for an error that tells the reader where to add
// one.
func registeredGoHomes() string {
	out := make([]string, 0, len(goHomes))
	for cuePath, home := range goHomes {
		out = append(out, cuePath+" => "+home.ImportPath)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
