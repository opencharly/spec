package retired

import (
	"os"
	"path/filepath"
	"testing"
)

// The org-wide sweep: EVERY surface a tree has, under the rules the
// contract's own scan applies to the surfaces it already reads.
//
// Each case below is a DISCRIMINATION: the tree is built so that the rule
// decides the verdict, and the case fails if the rule stops deciding it. A
// case that cannot fail proves nothing.

const (
	// unscopedName is withdrawn from every surface, so it is swept as a
	// TOKEN wherever it can appear.
	unscopedName = "podman_jobs"
	// scopedName is withdrawn from one Go type, so it is swept only where
	// that type is declared.
	scopedName  = "Build"
	scopedType  = "EngineConfig"
	scopedScope = "EngineConfig"
)

// writeTree materializes a tree from a map of repo-relative paths to
// contents, and returns its root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}
	return root
}

// namesOf returns the names the findings carry.
func namesOf(findings []Finding) []string {
	names := make([]string, 0, len(findings))
	for _, finding := range findings {
		names = append(names, finding.Identifier.Name)
	}
	return names
}

// carries reports whether findings carry name.
func carriesName(findings []Finding, name string) bool {
	for _, finding := range findings {
		if finding.Identifier.Name == name {
			return true
		}
	}
	return false
}

// A TEXT surface is swept. The contract's own scan reads Go, CUE and
// protobuf sources; an authored yaml, a skill's prose and a script are
// surfaces too, and a withdrawn name in one is just as live.
//
// The second assertion is the fail-without instrument: the contract's
// pre-existing scan finds NOTHING in this tree, so a control that used it
// would read the tree as clean.
func TestTheSweepReadsEveryTextSurface(t *testing.T) {
	root := writeTree(t, map[string]string{
		"candy/plugin-x/charly.yml": "defaults:\n  " + unscopedName + ": 4\n",
	})
	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if !carriesName(result.Findings, unscopedName) {
		t.Fatalf("the sweep must read an authored yaml: it carried no %q (read %v)", unscopedName, namesOf(result.Findings))
	}
	if result.Coverage.TextFiles != 1 {
		t.Errorf("the sweep read %d text file(s), want 1", result.Coverage.TextFiles)
	}

	before, err := ScanTree(root, Identifiers)
	if err != nil {
		t.Fatalf("scanning the tree with the contract's own scan: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("the pre-existing scan must miss this tree for the case to be a fail-without instrument, but it found %v", before)
	}
}

// A GO file is read STRUCTURALLY: a literal is a surface a consumer can
// reach, a comment is the record of the withdrawal.
func TestTheSweepReadsGoStructurally(t *testing.T) {
	root := writeTree(t, map[string]string{
		"candy/plugin-x/box.go": "package box\n\n// the knob " + unscopedName + " was renamed to stage_parallelism.\n" +
			"const knob = \"" + unscopedName + "\"\n",
	})
	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("the sweep found %d hit(s), want 1 — a comment is not a surface: %v", len(result.Findings), result.Findings)
	}
	if result.Findings[0].Detail != "a string literal" {
		t.Errorf("the hit is %q, want the literal", result.Findings[0].Detail)
	}
}

// A TEST file holds the assertions OF ABSENCE: the name is there on
// purpose, and its presence there is the proof, not the defect.
func TestTheSweepSkipsTestFiles(t *testing.T) {
	root := writeTree(t, map[string]string{
		"candy/plugin-x/box_test.go": "package box\n\nconst retired = \"" + unscopedName + "\"\n",
	})
	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the sweep found %v in a test file; a test file holds the assertions of absence", result.Findings)
	}
	if result.Coverage.SkippedTests != 1 {
		t.Errorf("the sweep skipped %d test file(s), want 1", result.Coverage.SkippedTests)
	}
}

// HISTORY is not a surface: a CHANGELOG entry that names a withdrawn
// identifier is the record of the withdrawal.
func TestTheSweepSkipsHistory(t *testing.T) {
	root := writeTree(t, map[string]string{
		"CHANGELOG/2026.1.1.md":       "# the knob " + unscopedName + " was renamed to stage_parallelism\n",
		"candy/plugin-x/CHANGELOG.md": "# " + unscopedName + "\n",
	})
	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the sweep found %v in history; history is the record of what happened", result.Findings)
	}
	if result.Coverage.SkippedHistory != 2 {
		t.Errorf("the sweep skipped %d history file(s), want 2", result.Coverage.SkippedHistory)
	}
}

// The file that PUBLISHES the names is excluded: it is where the names are
// supposed to appear.
func TestTheSweepExcludesTheList(t *testing.T) {
	root := writeTree(t, map[string]string{
		"retired/identifiers.json": `{"identifiers": [{"name": "` + unscopedName + `"}]}`,
	})
	result, err := SweepTree(root, Identifiers, SweepOptions{Exclude: []string{listPathForTest}})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the sweep found %v in the list that publishes the names", result.Findings)
	}
	if result.Coverage.Excluded != 1 {
		t.Errorf("the sweep excluded %d path(s), want 1", result.Coverage.Excluded)
	}
}

// listPathForTest is where the contract publishes the names, relative to
// the module root.
const listPathForTest = "retired/identifiers.json"

// A SCOPED name is swept where its scope is DECLARED, and nowhere else:
// a scope is a Go type, and a text file has no such notion.
func TestTheSweepHonoursTheScope(t *testing.T) {
	scoped := Identifier{Kind: KindField, Name: scopedName, Type: scopedType}
	declaring := "package box\n\ntype " + scopedScope + " struct {\n\t" + scopedName + " string\n}\n"
	notDeclaring := "package box\n\ntype Other struct {\n\t" + scopedName + " string\n}\n"

	declared := writeTree(t, map[string]string{"candy/plugin-x/box.go": declaring})
	result, err := SweepTree(declared, []Identifier{scoped}, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the declaring tree: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("the sweep found %d hit(s) where the scope is declared, want 1", len(result.Findings))
	}

	elsewhere := writeTree(t, map[string]string{"candy/plugin-x/box.go": notDeclaring})
	result, err = SweepTree(elsewhere, []Identifier{scoped}, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the other tree: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the sweep found %v where the scope is not declared; the name is legal there", result.Findings)
	}
}

// A file that is not TEXT is not a surface, and a nested repository is a
// repo of its own.
func TestTheSweepSkipsBinariesAndNestedRepos(t *testing.T) {
	root := writeTree(t, map[string]string{
		"candy/plugin-x/blob.bin": "engine",
		"vendor/thing/box.go":     "package box\n\nconst knob = \"" + unscopedName + "\"\n",
	})
	if err := os.WriteFile(filepath.Join(root, "candy", "plugin-x", "blob.bin"), []byte("engine\x00\x01"), 0o600); err != nil {
		t.Fatalf("writing the binary: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plugin-x"), 0o750); err != nil {
		t.Fatalf("creating the nested repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin-x", ".git"), []byte("gitdir: ../.git/modules/plugin-x\n"), 0o600); err != nil {
		t.Fatalf("writing the nested repo marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin-x", "box.go"), []byte("package box\n\nconst knob = \""+unscopedName+"\"\n"), 0o600); err != nil {
		t.Fatalf("writing the nested repo's file: %v", err)
	}

	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the sweep found %v; a binary and a nested repo are not this tree's surfaces", result.Findings)
	}
	if result.Coverage.SkippedBinary != 1 {
		t.Errorf("the sweep skipped %d binary file(s), want 1", result.Coverage.SkippedBinary)
	}
	if result.Coverage.SkippedNested != 1 {
		t.Errorf("the sweep skipped %d nested repo(s), want 1", result.Coverage.SkippedNested)
	}
}

// A SYMLINK is another PATH's content, not this tree's: it is not read, and
// a DANGLING one does not fail the run.
//
// Both halves discriminate. The link that points at a file inside the tree
// proves the content is not judged (a rule that read through the link would
// report the hit); the dangling link proves the run survives it (a rule that
// failed on an unreadable path would abort the whole sweep — and one broken
// link in one repo would fail the arm for every repo).
func TestTheSweepSkipsSymlinks(t *testing.T) {
	root := writeTree(t, map[string]string{
		"candy/plugin-x/box.go": "package box\n\nconst knob = \"" + unscopedName + "\"\n",
	})
	if err := os.Symlink("box.go", filepath.Join(root, "candy", "plugin-x", "link.go")); err != nil {
		t.Fatalf("creating the symlink: %v", err)
	}
	if err := os.Symlink("not-materialized", filepath.Join(root, "candy", "plugin-x", "dangling")); err != nil {
		t.Fatalf("creating the dangling symlink: %v", err)
	}

	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if result.Coverage.SkippedSymlinks != 2 {
		t.Errorf("the sweep skipped %d symlink(s), want 2", result.Coverage.SkippedSymlinks)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("the sweep found %d hit(s), want 1 — a link is another path's content: %v", len(result.Findings), result.Findings)
	}
	if result.Findings[0].File != "candy/plugin-x/box.go" {
		t.Errorf("the hit is in %q, want the file itself", result.Findings[0].File)
	}
}

// A hit is classified: a record covers it, and the hit is a RECORD. A
// record about ANOTHER identifier does not cover it, and a record that
// covers no hit is itself a finding.
func TestTheRecordsClassifyTheHits(t *testing.T) {
	root := writeTree(t, map[string]string{
		"candy/plugin-x/charly.yml": "defaults:\n  " + unscopedName + ": 4\n  other: 1\n",
	})
	result, err := SweepTree(root, Identifiers, SweepOptions{})
	if err != nil {
		t.Fatalf("sweeping the tree: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("the sweep found %d hit(s), want 1", len(result.Findings))
	}

	records := []Record{{
		Repo:       "opencharly/plugin-x",
		File:       "candy/plugin-x/charly.yml",
		Identifier: unscopedName,
		Contains:   unscopedName + ": 4",
		Why:        "the bed's negative arm: it exercises the retired key on purpose",
	}}
	verdict, err := Classify(root, result.Findings, records, "opencharly/plugin-x")
	if err != nil {
		t.Fatalf("classifying the hits: %v", err)
	}
	if len(verdict.Carriers) != 0 {
		t.Errorf("the hit is covered by a record, but it was called a carrier: %v", verdict.Carriers)
	}
	if len(verdict.Matches) != 1 {
		t.Errorf("the record matched %d hit(s), want 1", len(verdict.Matches))
	}
	if len(verdict.Stale) != 0 {
		t.Errorf("the record covers a hit, but it was called stale: %v", verdict.Stale)
	}

	// A record about ANOTHER identifier never excuses this one.
	other := []Record{{
		Repo:       "opencharly/plugin-x",
		File:       "candy/plugin-x/charly.yml",
		Identifier: "CHARLY_PODMAN_JOBS",
		Why:        "a record about another name",
	}}
	verdict, err = Classify(root, result.Findings, other, "opencharly/plugin-x")
	if err != nil {
		t.Fatalf("classifying the hits: %v", err)
	}
	if len(verdict.Carriers) != 1 {
		t.Errorf("a record about another identifier must not excuse this hit: %v", verdict.Carriers)
	}

	// A record whose TEXT does not match the hit's line does not cover it.
	moved := []Record{{
		Repo:       "opencharly/plugin-x",
		File:       "candy/plugin-x/charly.yml",
		Identifier: unscopedName,
		Contains:   "a line that is not there",
		Why:        "a record about a line that moved",
	}}
	verdict, err = Classify(root, result.Findings, moved, "opencharly/plugin-x")
	if err != nil {
		t.Fatalf("classifying the hits: %v", err)
	}
	if len(verdict.Carriers) != 1 {
		t.Errorf("a record whose text does not match must not cover the hit: %v", verdict.Carriers)
	}
	if len(verdict.Stale) != 1 {
		t.Errorf("a record that covers no hit is itself a finding: %v", verdict.Stale)
	}

	// A record about ANOTHER repository is neither a cover nor a stale
	// entry here: it is about another tree.
	elsewhere := []Record{{
		Repo:       "opencharly/other",
		File:       "candy/plugin-x/charly.yml",
		Identifier: unscopedName,
		Why:        "a record about another repository",
	}}
	verdict, err = Classify(root, result.Findings, elsewhere, "opencharly/plugin-x")
	if err != nil {
		t.Fatalf("classifying the hits: %v", err)
	}
	if len(verdict.Carriers) != 1 {
		t.Errorf("a record about another repository must not excuse this hit: %v", verdict.Carriers)
	}
	if len(verdict.Stale) != 0 {
		t.Errorf("a record about another repository is not stale here: %v", verdict.Stale)
	}
}

// A record may not invent a name, and it must say why.
func TestTheRecordsAreValidated(t *testing.T) {
	err := ValidateRecords([]Record{{Repo: "opencharly/x", File: "a.yml", Identifier: "a_name_the_contract_does_not_publish", Why: "why"}}, Identifiers)
	if err == nil {
		t.Error("a record about a name the contract does not publish must be rejected")
	}
	err = ValidateRecords([]Record{{Repo: "opencharly/x", File: "a.yml", Identifier: unscopedName}}, Identifiers)
	if err == nil {
		t.Error("a record without a reason must be rejected")
	}
	err = ValidateRecords([]Record{{File: "a.yml", Identifier: unscopedName, Why: "why"}}, Identifiers)
	if err == nil {
		t.Error("a record that names no repository must be rejected")
	}
}
