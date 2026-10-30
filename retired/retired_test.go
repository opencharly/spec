package retired

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The guard: no withdrawn identifier survives in the contract's own surfaces.
//
// This is the fail-without instrument for the class. It FAILS while a
// withdrawn name is present, which is what keeps a later "compatibility"
// commit from re-adding one — and it reads the names from the registry, so
// the next name withdrawn is guarded by the same instrument, with no second
// place to update.
func TestNoWithdrawnIdentifierInTheContract(t *testing.T) {
	root := moduleRoot(t)
	findings, err := ScanTree(root, Identifiers)
	if err != nil {
		t.Fatalf("scanning the contract: %v", err)
	}
	for _, finding := range findings {
		t.Errorf("%s", finding)
	}
}

// Every entry must be EFFECTIVE: the instrument must catch the name when it
// IS there. A key that cannot be made to fail proves nothing, and a vacuous
// key is this defect wearing a different hat.
func TestEveryIdentifierIsEffective(t *testing.T) {
	for _, id := range Identifiers {
		t.Run(string(id.Kind)+"/"+id.Name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "fixture.go")
			if err := os.WriteFile(path, []byte(plant(id)), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}
			findings, err := scanGoFile(path, "fixture.go", []Identifier{id})
			if err != nil {
				t.Fatalf("scanning the fixture: %v", err)
			}
			if len(findings) == 0 {
				t.Fatalf("the instrument did not catch the withdrawn %s %q — the key is vacuous", id.Kind, id.Name)
			}
		})
	}
}

// The registry's own rules. A withdrawn name is only useful if the entry says
// what surface it was withdrawn from, why it was withdrawn, what replaced it,
// and — where it names a type — that the type is one the contract has. A
// scope that names a type nobody declares is a rule that can never fire.
func TestTheRegistryIsWellFormed(t *testing.T) {
	root := moduleRoot(t)
	types, err := DeclaredTypes(root)
	if err != nil {
		t.Fatalf("reading the contract's declared types: %v", err)
	}
	for _, id := range Identifiers {
		switch id.Kind {
		case KindField, KindConfigKey, KindEnv, KindFlag:
		default:
			t.Errorf("the withdrawn %q has the unknown kind %q", id.Name, id.Kind)
			continue
		}
		if id.Name == "" {
			t.Errorf("a withdrawn identifier has no name: %+v", id)
		}
		if id.Why == "" {
			t.Errorf("the withdrawn %s says nothing about why it was withdrawn", id)
		}
		if id.ReplacedBy == "" {
			t.Errorf("the withdrawn %s names no replacement", id)
		}
		if id.Scoped() {
			if _, ok := types[id.Type]; !ok {
				t.Errorf("the withdrawn %s is scoped to the type %q, which the contract does not declare", id, id.Type)
				continue
			}
		}
		if err := replacementLanded(id, types); err != nil {
			t.Errorf("%v", err)
		}
	}
}

// replacementLanded judges an entry's replacement: a withdrawal whose
// replacement never landed is a rename that did not arrive.
//
// The Go surface is where the renames are exact, so the check is made there:
// the field the entry names must exist, in the scoped type or in some type
// when the name is unscoped; and a SCOPED configuration key's replacement
// must be a key its scoped type carries. An UNSCOPED key's replacement may
// be a path across types (`engine.run`), which no single type carries, so it
// is not judged here. The wire spellings ride on the same renames, and their
// replacements are owned by the repos that read them.
func replacementLanded(id Identifier, types map[string]TypeFields) error {
	switch id.Kind {
	case KindField:
		if id.Scoped() {
			if !carries(types[id.Type].Fields, id.ReplacedBy) {
				return fmt.Errorf("the withdrawn %s names the replacement %q, which %s does not carry", id, id.ReplacedBy, id.Type)
			}
			return nil
		}
		for _, info := range types {
			if carries(info.Fields, id.ReplacedBy) {
				return nil
			}
		}
		return fmt.Errorf("the withdrawn %s names the replacement %q, which no type in the contract carries", id, id.ReplacedBy)
	case KindConfigKey:
		if id.Scoped() && !carries(types[id.Type].Keys, id.ReplacedBy) {
			return fmt.Errorf("the withdrawn %s names the replacement %q, which %s does not carry as a configuration key", id, id.ReplacedBy, id.Type)
		}
	}
	return nil
}

// The replacement rule can FAIL. A rule that cannot fail proves nothing, and
// the rule is what tells a landed rename from a rename that never arrived.
func TestTheReplacementRuleCanFail(t *testing.T) {
	types := map[string]TypeFields{
		"EngineConfig": {Fields: []string{"Run"}, Keys: []string{"run"}},
	}
	landed := Identifier{Kind: KindField, Name: "Selector", Type: "EngineConfig", ReplacedBy: "Run"}
	if err := replacementLanded(landed, types); err != nil {
		t.Errorf("the rule must accept the replacement that landed: %v", err)
	}
	neverLanded := Identifier{Kind: KindField, Name: "Selector", Type: "EngineConfig", ReplacedBy: "Runn"}
	if err := replacementLanded(neverLanded, types); err == nil {
		t.Error("the rule must reject a field replacement that never landed")
	}
	neverLandedKey := Identifier{Kind: KindConfigKey, Name: "selector", Type: "EngineConfig", ReplacedBy: "runner"}
	if err := replacementLanded(neverLandedKey, types); err == nil {
		t.Error("the rule must reject a scoped configuration key whose replacement key is not carried")
	}
}

// plant returns a Go source carrying the withdrawn name in the form the
// entry's own rule catches.
func plant(id Identifier) string {
	switch id.Kind {
	case KindField:
		return "package fixture\n\ntype " + scopeOr(id, "Fixture") + " struct {\n\t" + id.Name + " string\n}\n"
	case KindConfigKey:
		return "package fixture\n\ntype " + scopeOr(id, "Fixture") + " struct {\n\tKept string `yaml:\"" + id.Name + "\"`\n}\n"
	case KindEnv, KindFlag:
		return "package fixture\n\nconst retired = \"" + id.Name + "\"\n"
	}
	return "package fixture\n"
}

// scopeOr returns the entry's scope, or fallback when it has none.
func scopeOr(id Identifier, fallback string) string {
	if id.Scoped() {
		return id.Type
	}
	return fallback
}

// carries reports whether names contains name.
func carries(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

// moduleRoot is the module's root directory — the tree the guard scans.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory — the guard would scan nothing")
		}
		dir = parent
	}
}
