package schemaconcat

import "testing"

// TestStripImportDecls pins the two forms CUE allows, and the block case especially: a parenthesised
// block must come back as ONE entry including its parentheses. Appending its lines individually and
// re-emitting them sorted would produce `import "list"` / `"strings"` — syntactically invalid — which
// is exactly the defect this test exists to prevent.
func TestStripImportDecls(t *testing.T) {
	t.Run("single-line import is removed and returned", func(t *testing.T) {
		body, imps := stripImportDecls([]byte("import \"list\"\n\n#A: {...list.MaxItems(2)}\n"))
		if len(imps) != 1 || imps[0] != "import \"list\"\n" {
			t.Fatalf("imports = %q, want the single-line declaration verbatim", imps)
		}
		if string(body) == "" || contains(string(body), "import") {
			t.Fatalf("body still carries the import: %q", body)
		}
	})

	t.Run("a parenthesised block stays ONE declaration", func(t *testing.T) {
		src := "import (\n\t\"list\"\n\t\"strings\"\n)\n\n#A: {}\n"
		body, imps := stripImportDecls([]byte(src))
		if len(imps) != 1 {
			t.Fatalf("imports = %d entries (%q), want ONE block", len(imps), imps)
		}
		if !contains(imps[0], "import (") || !contains(imps[0], ")") {
			t.Fatalf("the block lost its parentheses: %q", imps[0])
		}
		if !contains(imps[0], "\"list\"") || !contains(imps[0], "\"strings\"") {
			t.Fatalf("the block lost a member: %q", imps[0])
		}
		if contains(string(body), "import") {
			t.Fatalf("body still carries the import block: %q", body)
		}
	})

	t.Run("a file with no import is untouched", func(t *testing.T) {
		src := "#A: {x: 1}\n"
		body, imps := stripImportDecls([]byte(src))
		if len(imps) != 0 {
			t.Fatalf("imports = %q, want none", imps)
		}
		if string(body) != src {
			t.Fatalf("body changed: %q", body)
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
