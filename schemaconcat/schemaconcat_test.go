package schemaconcat

import (
	"testing"
	"testing/fstest"
)

// TestConcatSchemaStripsPackageClause is the gate on the package-form cutover's
// transitional half: schema/*.cue files now carry `package schema` (they are a
// real CUE package in the spec module), and the concatenated body every existing
// consumer compiles must stay the PACKAGE-LESS body it was. This fails on the
// pre-package implementation, which copied the clause into the joined body and
// therefore produced a body the CUE parser rejects ("multiple package clauses").
func TestConcatSchemaStripsPackageClause(t *testing.T) {
	fsys := fstest.MapFS{
		"schema/b.cue": {Data: []byte("package schema\n\n#B: string\n")},
		"schema/a.cue": {Data: []byte("package schema\n\n// a's header\n#A: string\n")},
	}
	body, names, err := ConcatSchema(fsys, "schema", nil)
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	wantBody := "// a's header\n#A: string\n\n#B: string\n\n"
	if body != wantBody {
		t.Fatalf("joined body = %q, want %q", body, wantBody)
	}
	if len(names) != 2 || names[0] != "a.cue" || names[1] != "b.cue" {
		t.Fatalf("names = %v, want the sorted [a.cue b.cue]", names)
	}
}

// TestConcatSchemaLeavesPackageLessBodyAlone pins the other direction: a schema
// directory that has NOT moved to a package (every plugin schema today) is
// concatenated exactly as before — the normalization must not rewrite a body
// that carries no clause.
func TestConcatSchemaLeavesPackageLessBodyAlone(t *testing.T) {
	src := "// header\n#A: string\n"
	fsys := fstest.MapFS{"params.cue": {Data: []byte(src)}}
	body, _, err := ConcatSchema(fsys, ".", nil)
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if body != src+"\n" {
		t.Fatalf("joined body = %q, want %q", body, src+"\n")
	}
}

// TestConcatSchemaIgnoresPackageField keeps the text normalization honest: a
// top-level FIELD named `package` (legal CUE, and not a clause) must survive,
// and a comment line before the clause must survive with it.
func TestConcatSchemaIgnoresPackageField(t *testing.T) {
	fsys := fstest.MapFS{"x.cue": {Data: []byte("package: string\n")}}
	body, _, err := ConcatSchema(fsys, ".", nil)
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if body != "package: string\n\n" {
		t.Fatalf("joined body = %q, want the field untouched", body)
	}

	// A clause with NO blank separator: the blank line is optional, so only the
	// clause line goes and the body starts at the first declaration.
	fsys = fstest.MapFS{"c.cue": {Data: []byte("package schema\n#C: string\n")}}
	body, _, err = ConcatSchema(fsys, ".", nil)
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if body != "#C: string\n\n" {
		t.Fatalf("joined body = %q, want the clause line gone and no separator consumed", body)
	}

	fsys = fstest.MapFS{"y.cue": {Data: []byte("// lead\npackage schema\n\n#Y: string\n")}}
	body, _, err = ConcatSchema(fsys, ".", nil)
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if body != "// lead\n#Y: string\n\n" {
		t.Fatalf("joined body = %q, want the comment kept and the clause stripped", body)
	}
}

// TestConcatSchemaExcludeAndNonCue covers the two filters the callers rely on:
// exclude drops a named file, and a non-.cue entry is never read.
func TestConcatSchemaExcludeAndNonCue(t *testing.T) {
	fsys := fstest.MapFS{
		"schema/keep.cue": {Data: []byte("package schema\n#Keep: string\n")},
		"schema/drop.cue": {Data: []byte("package schema\n#Drop: string\n")},
		"schema/notes.md": {Data: []byte("not a schema")},
	}
	body, names, err := ConcatSchema(fsys, "schema", func(n string) bool { return n == "drop.cue" })
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	if body != "#Keep: string\n\n" || len(names) != 1 || names[0] != "keep.cue" {
		t.Fatalf("body=%q names=%v, want only keep.cue's body", body, names)
	}
}
