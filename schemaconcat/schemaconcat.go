// THE single schema-concatenation contract (R3).
//
// Both the runtime (charly core (cue_schema.go) `sharedCueSchema`) and the dev-time
// code generator (internal/schemagen) need to concatenate every schema/*.cue
// file into ONE package-less compilation unit. They used to carry two
// byte-identical-by-comment copies of the same loop; this is the ONE copy they
// both call, so the compiled schema (what the runtime validates against) and
// the generated Go types (`schemaparams`) can never drift.
//
// The schema files are ALSO a real CUE package in the `github.com/opencharly/spec`
// module (`cue.mod/module.cue`; `package schema`, loaded by `cue eval ./schema`).
// The text splice and the package are one schema in two forms during the
// cutover: the splice stays the runtime path until charly imports the package,
// so ConcatSchema normalizes a file's package clause away and the joined body it
// returns is unchanged, byte for byte, from the pre-package one.
//
// Leaf package (no deps beyond stdlib `io/fs`) so the dev-time generator can
// call it WITHOUT importing the spec package it regenerates. That is why the
// clause is stripped textually rather than by parsing: parsing would make this
// leaf take cuelang.org/go for what is a line deletion.
//
// STANDALONE: depends only on the stdlib `io/fs` abstraction, so the runtime
// passes its `//go:embed` FS and the generator passes `os.DirFS(schemaDir)`.
package schemaconcat

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// ConcatSchema reads every `*.cue` file in fsys under dir (skipping any for
// which exclude reports true — pass nil to include all), sorts the names for a
// deterministic result, and joins the file bodies with a trailing newline. It
// returns the joined body (WITHOUT any package clause) and the sorted list of
// files included. A file's leading `package <name>` declaration — the schema is
// a CUE package in the spec module — is removed, together with the single blank
// line that conventionally separates it from the first declaration; every other
// byte is preserved.
func ConcatSchema(fsys fs.FS, dir string, exclude func(name string) bool) (string, []string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", nil, fmt.Errorf("read schema dir %q: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cue") {
			continue
		}
		if exclude != nil && exclude(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names) // deterministic concatenation
	var b strings.Builder
	for _, n := range names {
		data, err := fs.ReadFile(fsys, path(dir, n))
		if err != nil {
			return "", nil, fmt.Errorf("read schema %s: %w", n, err)
		}
		b.Write(stripPackageClause(data))
		b.WriteString("\n")
	}
	return b.String(), names, nil
}

// path joins a dir and a file name with "/" (fs.FS always uses forward slashes,
// never the OS separator). A "." dir yields the bare name.
func path(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}

// stripPackageClause removes a schema file's leading `package <name>`
// declaration so ConcatSchema keeps returning the package-less body its callers
// compile. Only the first non-blank, non-`//` line is considered: a package
// clause must be the file's first declaration, so anything else at that point
// means the file carries no clause (the pre-package form, and every plugin
// schema that has not moved to a module) and it is returned untouched.
func stripPackageClause(src []byte) []byte {
	offset := 0
	for offset < len(src) {
		end := bytes.IndexByte(src[offset:], '\n')
		var line []byte
		if end < 0 {
			line = src[offset:]
		} else {
			line = src[offset : offset+end]
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte("//")) {
			if end < 0 {
				return src
			}
			offset += end + 1
			continue
		}
		if !isPackageClause(trimmed) {
			return src
		}
		if end < 0 {
			return src[:offset]
		}
		rest := offset + end + 1
		// The blank line that conventionally separates the clause from the file's
		// first declaration belongs to the clause, not to the body: dropping it too
		// keeps the joined body byte-identical to the pre-package one.
		if rest < len(src) && src[rest] == '\n' {
			rest++
		}
		stripped := make([]byte, 0, len(src)-(rest-offset))
		stripped = append(stripped, src[:offset]...)
		return append(stripped, src[rest:]...)
	}
	return src
}

// isPackageClause reports whether a trimmed source line is a `package <name>`
// declaration (the `package` keyword followed by whitespace, never a field
// named `package`).
func isPackageClause(line []byte) bool {
	const keyword = "package"
	if !bytes.HasPrefix(line, []byte(keyword)) {
		return false
	}
	rest := line[len(keyword):]
	return len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t')
}
