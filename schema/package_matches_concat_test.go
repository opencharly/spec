package schema_test

import (
	"path/filepath"
	"sort"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// definitionNames returns every top-level definition a value carries.
func definitionNames(t *testing.T, v cue.Value) []string {
	t.Helper()
	it, err := v.Fields(cue.All())
	if err != nil {
		t.Fatalf("iterating fields: %v", err)
	}
	var names []string
	for it.Next() {
		sel := it.Selector()
		if sel.IsDefinition() {
			names = append(names, sel.String())
		}
	}
	sort.Strings(names)
	return names
}

// TestPackageCarriesTheWholeSchema gates the cutover's two forms against each
// other: the SAME schema files are compiled as the CUE package a module loads
// (`cue eval ./schema`, what a consumer imports) and as the package-less
// concatenation the runtime and the generator still compile. Every top-level
// definition must exist in BOTH, with the same value — so a file the package
// loader excludes (a name CUE ignores, such as a leading `_`) or a clause the
// concatenator stops stripping fails here.
func TestPackageCarriesTheWholeSchema(t *testing.T) {
	body, files, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("the concatenation read no schema files")
	}
	ctx := cuecontext.New()
	concat := ctx.CompileString(body)
	if err := concat.Err(); err != nil {
		t.Fatalf("the concatenated schema does not compile: %v", err)
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolving the module root: %v", err)
	}
	instances := load.Instances([]string{"./schema"}, &load.Config{Dir: root})
	if len(instances) != 1 {
		t.Fatalf("loading ./schema from %s produced %d instances, want 1", root, len(instances))
	}
	if err := instances[0].Err; err != nil {
		t.Fatalf("loading the schema package: %v", err)
	}
	pkg := ctx.BuildInstance(instances[0])
	if err := pkg.Err(); err != nil {
		t.Fatalf("the schema package does not compile: %v", err)
	}

	concatNames := definitionNames(t, concat)
	pkgNames := definitionNames(t, pkg)
	if len(concatNames) == 0 {
		t.Fatal("the concatenated schema carries no definitions")
	}
	if len(concatNames) != len(pkgNames) {
		t.Fatalf("the concatenation has %d definitions, the package %d — a file the package loader excludes?",
			len(concatNames), len(pkgNames))
	}
	for i := range concatNames {
		if concatNames[i] != pkgNames[i] {
			t.Fatalf("definition sets diverge at %d: concatenation %q, package %q",
				i, concatNames[i], pkgNames[i])
		}
	}

	for _, name := range concatNames {
		path := cue.ParsePath(name)
		a := concat.LookupPath(path)
		b := pkg.LookupPath(path)
		if !a.Exists() || !b.Exists() {
			t.Fatalf("%s: exists in concatenation=%v, package=%v", name, a.Exists(), b.Exists())
		}
		// Compare the rendered value, not Subsumption: a definition that a
		// `close()` or an explicit `_|_` arm makes closed is not subsumed
		// symmetrically even when the two values are identical, so the rendered
		// form is the comparison that means "the same thing".
		ar, err := format.Node(a.Syntax())
		if err != nil {
			t.Fatalf("%s: rendering the concatenation's value: %v", name, err)
		}
		br, err := format.Node(b.Syntax())
		if err != nil {
			t.Fatalf("%s: rendering the package's value: %v", name, err)
		}
		if string(ar) != string(br) {
			t.Fatalf("%s: the package's value differs from the concatenation's:\npackage:\n%s\nconcatenation:\n%s",
				name, truncate(string(br)), truncate(string(ar)))
		}
	}
}

// truncate bounds a failing comparison's output so the test log stays readable.
func truncate(s string) string {
	const limit = 1200
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n… (truncated)"
}
