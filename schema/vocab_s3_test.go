package schema_test

// vocab_s3_test.go — the §10.1 S3 gate over the shipped schema.
//
// Every closed set of words in this schema is a vocabulary table (`_x: {word: {},
// ...} @vocab(owner)`), its word list is DERIVED from that table
// (`_xWord: [for w, _ in _x {w}]`), and the enum an author meets is `or(_xWord)`
// (D-PAT-3's form half; DESIGN.md §10.1 S3). That shape is what makes "a new word is
// ONE row" true: the table, the derived list and the field's accepted set cannot
// disagree, and no file restates another's word list.
//
// The gate reads the SOURCE through the CUE parser (so it can see the `or(...)` a
// field cites and the `@vocab` attribute a table carries — neither survives in the
// compiled value) and the VALUE through the runtime's own concatenation
// (schemaconcat.ConcatSchema, the same body charly core's sharedCueSchema compiles),
// so it judges the schema that ships rather than a fixture.
//
// A table whose derived list is checked against itself proves nothing, so the
// mutation half INJECTS a row into every table and asserts the derived list — and
// therefore the enum — gains it. Each mutation first proves it APPLIED: the injected
// word must be absent from the unmutated list and present after.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// vocabTable is ONE table found in the source: its label, its rows (in declaration
// order), and the attribute the declaration carries.
type vocabTable struct {
	name  string // the label without the leading "_", e.g. "context"
	rows  []string
	owner string
	file  string
	line  int
}

// vocabSite is ONE `or(_xWord)` citation: which file/line cites it and which table
// name it derives from.
type vocabSite struct {
	table string
	file  string
	line  int
	// inside is the table whose ROWS carry this citation, or "" when the citation is a field's
	// own constraint. A table cited from inside another table's rows is a ROW of that table, not
	// a standalone vocabulary — which is why it may legitimately hold a single word (see
	// TestEveryTableDerivesItsWordList).
	inside string
}

// concatSchema returns the package-less body the runtime and the generator share.
func concatSchema(t *testing.T) string {
	t.Helper()
	src, files, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("the shipped schema concatenated to zero files")
	}
	return src
}

// scanVocabulary walks every shipped schema file and returns the tables it declares
// and the `or(_xWord)` citations it carries.
func scanVocabulary(t *testing.T) (map[string]vocabTable, []vocabSite) {
	tables, sites, _ := scanVocabularyFull(t)
	return tables, sites
}

// scanVocabularyFull is scanVocabulary plus the word lists defined as a COMPREHENSION over another
// table (`_leasableWord: [for w, r in _capability if r.lease {w}]`). Those are derived lists, not
// tables: they have no rows of their own to pin, and the gate must resolve a citation of them
// rather than report "no file declares it".
func scanVocabularyFull(t *testing.T) (map[string]vocabTable, []vocabSite, map[string]bool) {
	t.Helper()
	entries, err := schema.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read schema dir: %v", err)
	}
	tables := map[string]vocabTable{}
	derived := map[string]bool{}
	var sites []vocabSite
	var iterated []vocabSite
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cue") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := schema.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		f, err := parser.ParseFile(name, raw, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Walk(f, func(n ast.Node) bool {
			fld, ok := n.(*ast.Field)
			if !ok {
				return true
			}
			label := strings.Trim(labelText(t, fld.Label), `"`)
			inside := ""
			if strings.HasPrefix(label, "_") && !strings.HasSuffix(label, "Word") {
				if rows, ok := rowLabels(t, fld.Value); ok {
					key := strings.TrimPrefix(label, "_")
					if prev, dup := tables[key]; dup {
						t.Errorf("%s:%d and %s:%d both declare _%s — a table is one home (D-PAT-2)",
							prev.file, prev.line, name, fld.Pos().Line(), key)
					}
					tables[key] = vocabTable{
						name: key, rows: rows, owner: vocabOwner(fld), file: name, line: fld.Pos().Line(),
					}
					inside = key
				}
			}
			// a `_xWord` defined as a list comprehension over another table
			if strings.HasPrefix(label, "_") && strings.HasSuffix(label, "Word") {
				if lit, ok := fld.Value.(*ast.ListLit); ok && len(lit.Elts) == 1 {
					if _, ok := lit.Elts[0].(*ast.Comprehension); ok {
						derived[strings.TrimSuffix(strings.TrimPrefix(label, "_"), "Word")] = true
					}
				}
			}
			// every `or(_xWord)` citation, wherever it appears
			ast.Walk(fld.Value, func(c ast.Node) bool {
				// a `for k, v in _x` COMPREHENSION cites the table it iterates, exactly as
				// `or(_xWord)` cites it: `_role`'s rows reach `_capability` through
				// `for r, _ in _role {…}`, and `_capability`'s rows reach `#Need` through
				// `for t, c in _capability if …`. Without this the gate reads both as dead.
				if comp, ok := c.(*ast.Comprehension); ok && len(comp.Clauses) > 0 {
					if fc, ok := comp.Clauses[0].(*ast.ForClause); ok {
						src := strings.Trim(labelText(t, fc.Source), `"`)
						if strings.HasPrefix(src, "_") && !strings.HasSuffix(src, "Word") {
							iterated = append(iterated, vocabSite{
								table: strings.TrimPrefix(src, "_"), file: name,
								line: comp.Pos().Line(), inside: inside,
							})
						}
					}
				}
				call, ok := c.(*ast.CallExpr)
				if !ok {
					return true
				}
				if fn := labelText(t, call.Fun); fn != "or" || len(call.Args) != 1 {
					return true
				}
				ident := labelText(t, call.Args[0])
				if !strings.HasPrefix(ident, "_") || !strings.HasSuffix(ident, "Word") {
					return true
				}
				sites = append(sites, vocabSite{
					table:  strings.TrimSuffix(strings.TrimPrefix(ident, "_"), "Word"),
					file:   name,
					line:   call.Pos().Line(),
					inside: inside,
				})
				return true
			}, nil)
			return true
		}, nil)
	}
	// A comprehension over something that is NOT a table (`for w, _ in _xWord`) cites a list, not a
	// table, and must not be reported as a dangling citation — so the candidates are kept only
	// when the table they name really exists.
	for _, s := range iterated {
		if _, ok := tables[s.table]; ok {
			sites = append(sites, s)
		}
	}
	return tables, sites, derived
}

// labelText renders a label/expression's identifier text.
func labelText(t *testing.T, n ast.Node) string {
	t.Helper()
	switch x := n.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.BasicLit:
		return x.Value
	case *ast.SelectorExpr:
		return labelText(t, x.X) + "." + labelText(t, x.Sel)
	}
	return ""
}

// rowLabels returns the regular field labels of a table literal, or ok=false when
// the value is not a struct literal of rows.
func rowLabels(t *testing.T, e ast.Expr) ([]string, bool) {
	t.Helper()
	lit, ok := e.(*ast.StructLit)
	if !ok {
		return nil, false
	}
	var out []string
	for _, el := range lit.Elts {
		// A table's literal may carry more than plain rows: a row PATTERN (`[#Word]: #NativeRow`,
		// which makes every row the row type) and a COMPREHENSION (`for r, _ in _role {…}`, which
		// derives rows from another table). Neither is a row of its own — `cue` does not iterate a
		// pattern into the derived list, and a comprehension's rows are not AST fields — so the
		// label set is compared against the table's EVALUATED keys instead (see
		// TestEveryTableDerivesItsWordList), not against this list.
		fld, ok := el.(*ast.Field)
		if !ok {
			continue
		}
		lbl := strings.Trim(labelText(t, fld.Label), `"`)
		if lbl == "" {
			continue
		}
		out = append(out, lbl)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// vocabOwner returns the value of the declaration's `@vocab(...)` attribute. The
// attribute is carried as TEXT (`@vocab(spec)`), so it is read by splitting it rather
// than by walking a body the AST does not expose.
func vocabOwner(fld *ast.Field) string {
	for _, a := range fld.Attrs {
		key, body := a.Split()
		if key != "vocab" {
			continue
		}
		return strings.TrimSpace(body)
	}
	return ""
}

// compiledSchema compiles the concatenation — the runtime's own composition.
func compiledSchema(t *testing.T, src string) cue.Value {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("the shipped schema does not compile: %v", err)
	}
	return v
}

func decodeLists(t *testing.T, tables map[string]vocabTable, body string) map[string][]string {
	t.Helper()
	v := compiledSchema(t, body+harnessFor(tables))
	var out struct {
		Words map[string][]string `json:"words"`
	}
	if err := v.LookupPath(cue.ParsePath("#TestVocabulary")).Decode(&out); err != nil {
		t.Fatalf("lift the derived word lists: %v", err)
	}
	return out.Words
}

// decodeKeys lifts each table's own evaluated key set.
func decodeKeys(t *testing.T, tables map[string]vocabTable, body string) map[string][]string {
	t.Helper()
	v := compiledSchema(t, body+harnessFor(tables))
	var out struct {
		Keys map[string][]string `json:"keys"`
	}
	if err := v.LookupPath(cue.ParsePath("#TestVocabulary")).Decode(&out); err != nil {
		t.Fatalf("lift the tables' key sets: %v", err)
	}
	return out.Keys
}

// sameWords reports whether two word lists hold the same words (order aside).
func sameWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// TestEveryTableDerivesItsWordList asserts the S3 shape holds everywhere: every
// table carries an @vocab owner, every table's derived list IS the table's own key
// set, and every STANDALONE table has more than one row.
func TestEveryTableDerivesItsWordList(t *testing.T) {
	tables, sites := scanVocabulary(t)
	if len(tables) == 0 {
		t.Fatal("no vocabulary tables found — the scan would pass over nothing")
	}
	if len(sites) == 0 {
		t.Fatal("no `or(_xWord)` citation found — the scan would pass over nothing")
	}
	src := concatSchema(t)
	lists := decodeLists(t, tables, src)
	keys := decodeKeys(t, tables, src)

	// A table cited from INSIDE another table's rows is a ROW of that table, not a standalone
	// vocabulary: `_isolation: {microvm}` has one legal value because the capability has one
	// implementation, and `_capability`'s row is where that value is asked for. The single-row
	// rule stays for every table that stands on its own.
	rowOf := map[string]bool{}
	for _, s := range sites {
		if s.inside != "" && s.inside != s.table {
			rowOf[s.table] = true
		}
	}

	for name, tbl := range tables {
		if tbl.owner != "spec" {
			t.Errorf("%s:%d: table _%s carries @vocab(%q); every set in this module is compiled into the `spec` contract module, so the owner is `spec`",
				tbl.file, tbl.line, name, tbl.owner)
		}
		if len(tbl.rows) < 2 && !rowOf[name] {
			t.Errorf("%s:%d: table _%s has %d row(s) — a one-word table is not a closed set of words, it is a constant (a table cited from another table's rows is exempt: it is that table's row, not a vocabulary)",
				tbl.file, tbl.line, name, len(tbl.rows))
		}
		got, ok := lists[name]
		if !ok {
			t.Errorf("table _%s has no derived _%sWord list in the compiled schema", name, name)
			continue
		}
		want, ok := keys[name]
		if !ok {
			t.Errorf("table _%s does not evaluate to a struct of rows", name)
			continue
		}
		if !sameWords(got, want) {
			t.Errorf("_%sWord is %v but the _%s table's own keys are %v — the list is restated, not derived",
				name, got, name, want)
		}
	}
}

// TestEveryTableIsCitedAndEveryCitationResolves asserts the wiring is one-to-one: a
// table nothing cites is dead (its rows constrain nothing), and a citation of a table
// that does not exist is a compile error the next reader would have to find.
func TestEveryTableIsCitedAndEveryCitationResolves(t *testing.T) {
	tables, sites, derived := scanVocabularyFull(t)
	cited := map[string]bool{}
	for _, s := range sites {
		cited[s.table] = true
		_, isTable := tables[s.table]
		if !isTable && !derived[s.table] {
			t.Errorf("%s:%d cites _%sWord, which no file declares", s.file, s.line, s.table)
		}
	}
	for name, tbl := range tables {
		if !cited[name] {
			t.Errorf("%s:%d: table _%s is cited by no field or definition — its rows constrain nothing",
				tbl.file, tbl.line, name)
		}
	}
}

// TestInjectedRowReachesTheDerivedList mutates EVERY table: a row appended to the
// table must appear in the derived list, which is the only way a runtime test can see
// that the list DERIVES rather than restates. Equality cannot see it — a hand-written
// list has exactly the table's words — so each mutation also proves it APPLIED (the
// injected word is absent before and present after).
func TestInjectedRowReachesTheDerivedList(t *testing.T) {
	const injected = "probe-injected-word"
	tables, _ := scanVocabulary(t)
	base := concatSchema(t)
	before := decodeLists(t, tables, base)
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if contains(before[name], injected) {
			t.Fatalf("the unmutated _%sWord already carries %q — the mutation would be unprovable here", name, injected)
		}
		src := base + fmt.Sprintf("\n_%s: {%q: {}}\n", name, injected)
		after := decodeLists(t, tables, src)
		if !contains(after[name], injected) {
			t.Errorf("a row was injected into _%s and its derived list did not gain %q — _%sWord does not derive from the table",
				name, injected, name)
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// harnessFor renders the lifting definition for the tables the scan found.
func harnessFor(tables map[string]vocabTable) string {
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("\n#TestVocabulary: {words: {\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "\t%s: _%sWord\n", k, k)
	}
	// The table's own keys, evaluated. The AST cannot see the rows a COMPREHENSION yields
	// (`for r, _ in _role {…}`) nor iterate a row PATTERN, so the "the list derives from the
	// table" question is asked of the table's value, not of its source text.
	b.WriteString("}, keys: {\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "\t%s: [for w, _ in _%s {w}]\n", k, k)
	}
	b.WriteString("}}\n")
	return b.String()
}
