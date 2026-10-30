package retired

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Finding is one occurrence of a withdrawn identifier in a scanned file.
type Finding struct {
	// Identifier is the withdrawn name that was found.
	Identifier Identifier
	// File is the path of the file, relative to the scanned root.
	File string
	// Line is the 1-indexed line the occurrence is on.
	Line int
	// Detail says what was matched — a declaration, a tag, a literal.
	Detail string
}

// String renders the finding for a report.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: the withdrawn %s %s is present (%s)",
		f.File, f.Line, f.Identifier.Kind, f.Identifier, f.Detail)
}

// ScanTree reports every withdrawn identifier the tree under root carries.
//
// The tree is scanned BY SURFACE: Go sources contribute their struct
// declarations and their string literals; CUE and protobuf sources contribute
// their text. Two kinds of file are skipped, each for a reason that is part of
// the rule rather than an exception to it:
//
//   - a TEST file holds the assertions OF ABSENCE — the name is there on
//     purpose, and its presence there is the proof, not the defect;
//   - a testdata directory holds fixtures, which are inputs to a test and not
//     surfaces of the contract.
//
// A GENERATED file is NOT skipped. It is a committed surface — it is what
// every consumer compiles against — so a withdrawn name in it is a real
// finding. It is derived from an authored source, so the withdrawal is
// normally visible in both; the guard reports both, because they are two
// files a fix must touch: the authored source AND the artifact it generates.
//
// The Go scan is STRUCTURAL: it reads declarations and literals, never
// comments. A Go comment naming a withdrawn identifier is a record of the
// withdrawal — the contract documents what it removed, and that documentation
// is correct — while a declaration or a literal is a surface a consumer can
// still reach. The CUE scan is textual, comments included: the schema's prose
// is part of the schema's published surface, so a withdrawn key named there is
// stale documentation of a key that is gone.
func ScanTree(root string, ids []Identifier) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !scannableFile(path) {
			return nil
		}
		found, scanErr := scanFile(path, rel, ids)
		if scanErr != nil {
			return scanErr
		}
		findings = append(findings, found...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}

// TypeFields is what a struct type declares: its field names, and the
// yaml/json keys those fields carry. The registry's replacement rule reads
// both: a withdrawn FIELD is replaced by a field, and a withdrawn
// configuration key by a key.
type TypeFields struct {
	Fields []string
	Keys   []string
}

// DeclaredTypes returns what every struct type in the tree declares, keyed by
// type name. It answers the "does this type exist, and what does it carry"
// question that the registry's own well-formedness rule asks.
//
// The generated files are part of the answer: the generated types are the
// contract's published wire types, and "did the replacement land" is a
// question about them.
func DeclaredTypes(root string) (map[string]TypeFields, error) {
	types := map[string]TypeFields{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		declared, declErr := declaredTypes(path)
		if declErr != nil {
			return declErr
		}
		for name, info := range declared {
			merged := types[name]
			merged.Fields = append(merged.Fields, info.Fields...)
			merged.Keys = append(merged.Keys, info.Keys...)
			types[name] = merged
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return types, nil
}

// scanFile dispatches on the file's surface.
func scanFile(path, rel string, ids []Identifier) ([]Finding, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return scanGoFile(path, rel, ids)
	case ".cue", ".proto":
		return scanTextFile(path, rel, ids)
	}
	return nil, nil
}

// scanGoFile reports the withdrawn identifiers a Go source file carries.
func scanGoFile(path, rel string, ids []Identifier) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rel, err)
	}

	// The file's struct types are walked once: the tag rule and the field
	// rule both need them, and re-deriving them would read the same
	// declarations twice for no gain.
	structs := structTypes(file)

	// A struct tag is checked by the TAG rule, which knows the field's type.
	// The literal rule skips the tags, so one withdrawn key is reported once
	// and not twice.
	tags := map[token.Pos]bool{}
	for _, structType := range structs {
		for _, field := range structType.structType.Fields.List {
			if field.Tag != nil {
				tags[field.Tag.Pos()] = true
			}
		}
	}

	var findings []Finding
	for _, structType := range structs {
		findings = append(findings, scanStruct(fset, rel, structType.typeName, structType.structType, ids)...)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || tags[lit.Pos()] {
			return true
		}
		value, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil {
			return true
		}
		findings = append(findings, scanLiteral(fset, rel, lit, value, ids)...)
		return true
	})
	return findings, nil
}

// scanStruct reports the withdrawn identifiers a struct declaration carries: a
// withdrawn FIELD name, and a withdrawn configuration KEY carried by a field's
// yaml/json tag.
func scanStruct(fset *token.FileSet, rel, typeName string, structType *ast.StructType, ids []Identifier) []Finding {
	var findings []Finding
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			for _, id := range ids {
				if id.Kind != KindField || id.Name != name.Name {
					continue
				}
				if id.Scoped() && id.Type != typeName {
					continue
				}
				findings = append(findings, Finding{
					Identifier: id,
					File:       rel,
					Line:       fset.Position(name.Pos()).Line,
					Detail:     "a declared field of " + typeName,
				})
			}
		}
		if field.Tag == nil {
			continue
		}
		for _, name := range fieldKeys(field) {
			for _, id := range ids {
				if id.Kind != KindConfigKey || id.Name != name {
					continue
				}
				if id.Scoped() && id.Type != typeName {
					continue
				}
				findings = append(findings, Finding{
					Identifier: id,
					File:       rel,
					Line:       fset.Position(field.Tag.Pos()).Line,
					Detail:     "the configuration key of a field of " + typeName,
				})
			}
		}
	}
	return findings
}

// scanLiteral reports the withdrawn wire spellings a string literal carries:
// an environment variable, a command-line flag, or an unscoped configuration
// key.
func scanLiteral(fset *token.FileSet, rel string, lit *ast.BasicLit, value string, ids []Identifier) []Finding {
	var findings []Finding
	for _, id := range ids {
		if id.Kind != KindConfigKey && id.Kind != KindEnv && id.Kind != KindFlag {
			continue
		}
		// A scoped name is matched inside its scope only; a bare token match
		// for a short word like `run` would match inside `runtime`.
		if id.Scoped() {
			continue
		}
		if !containsToken(value, id.Name) {
			continue
		}
		findings = append(findings, Finding{
			Identifier: id,
			File:       rel,
			Line:       fset.Position(lit.Pos()).Line,
			Detail:     "a string literal",
		})
	}
	return findings
}

// scanTextFile reports the withdrawn identifiers an authored CUE or protobuf
// source carries. The whole text is swept, comments included: the schema's
// prose is part of the schema's published surface.
func scanTextFile(path, rel string, ids []Identifier) ([]Finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", rel, err)
	}
	text := string(data)
	var findings []Finding
	for _, id := range ids {
		// A scope is a Go type, and a CUE or protobuf file has no such notion,
		// so a scoped name is not swept here.
		if id.Scoped() {
			continue
		}
		if !containsToken(text, id.Name) {
			continue
		}
		findings = append(findings, Finding{
			Identifier: id,
			File:       rel,
			Line:       lineOf(text, id.Name),
			Detail:     "named in the source text",
		})
	}
	return findings, nil
}

// declaredTypes returns what every struct type a Go file declares.
func declaredTypes(path string) (map[string]TypeFields, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	types := map[string]TypeFields{}
	for _, structType := range structTypes(file) {
		info := types[structType.typeName]
		for _, field := range structType.structType.Fields.List {
			for _, name := range field.Names {
				info.Fields = append(info.Fields, name.Name)
			}
			info.Keys = append(info.Keys, fieldKeys(field)...)
		}
		types[structType.typeName] = info
	}
	return types, nil
}

// fieldKeys returns the configuration keys a field carries: the yaml key and
// the json key, without their options. A key that both dialects spell the
// same way is ONE key — the dialects are two spellings of one authored key, so
// a withdrawal of it is one occurrence, not two.
func fieldKeys(field *ast.Field) []string {
	if field.Tag == nil {
		return nil
	}
	// The AST carries the tag as the SOURCE literal, so it still carries its
	// quotes. reflect.StructTag reads a leading quote as part of the key's
	// name, so the literal is unquoted first.
	value, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		value = strings.Trim(field.Tag.Value, "`")
	}
	tag := reflect.StructTag(value)
	var keys []string
	for _, key := range []string{"yaml", "json"} {
		value, ok := tag.Lookup(key)
		if !ok {
			continue
		}
		name := strings.Split(value, ",")[0]
		if slices.Contains(keys, name) {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

// namedStruct pairs a struct type with the name of the type it declares.
type namedStruct struct {
	typeName   string
	structType *ast.StructType
}

// structTypes returns the struct types a file declares, each paired with the
// name of the type it is declared as.
func structTypes(file *ast.File) []namedStruct {
	var structs []namedStruct
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			structs = append(structs, namedStruct{typeName: typeSpec.Name.Name, structType: structType})
		}
	}
	return structs
}

// scannableFile reports whether a file is one of the contract's surfaces: a
// Go source that is not a test, a CUE schema, or a protobuf schema.
func scannableFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cue", ".proto":
		return true
	case ".go":
		return !strings.HasSuffix(path, "_test.go")
	}
	return false
}

// skipDir reports whether a directory holds nothing the contract's surfaces
// live in.
func skipDir(name string) bool {
	switch name {
	case ".git", "testdata", "vendor", "node_modules":
		return true
	}
	return false
}

// containsToken reports whether text contains name as a TOKEN: the characters
// around the match are not word characters, so a short name does not match
// inside a longer one that starts with it, and one that ends in an underscore
// does not match inside the name that extends it.
func containsToken(text, name string) bool {
	return tokenIndex(text, name) >= 0
}

// tokenIndex returns the index of the first TOKEN occurrence of name in text,
// or -1 when there is none.
//
// The TOKEN rule lives here and nowhere else, so the predicate and the line
// reporter cannot disagree about WHICH occurrence matched: a name whose first
// substring sits inside a longer word (`run` in `runtime`) is not a match,
// and reporting the line of that substring would point the reader at a place
// the name does not occur as a token.
func tokenIndex(text, name string) int {
	if name == "" || len(name) > len(text) {
		return -1
	}
	for from := 0; from+len(name) <= len(text); {
		i := strings.Index(text[from:], name)
		if i < 0 {
			return -1
		}
		i += from
		if !isWordByte(byteAt(text, i-1)) && !isWordByte(byteAt(text, i+len(name))) {
			return i
		}
		from = i + 1
	}
	return -1
}

// lineOf returns the 1-indexed line the first TOKEN occurrence of name is on.
func lineOf(text, name string) int {
	i := tokenIndex(text, name)
	if i < 0 {
		return 0
	}
	return strings.Count(text[:i], "\n") + 1
}

// byteAt returns the byte at i, or 0 when i is outside the string.
func byteAt(text string, i int) byte {
	if i < 0 || i >= len(text) {
		return 0
	}
	return text[i]
}

// isWordByte reports whether b is a letter, a digit or an underscore.
func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
