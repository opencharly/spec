package retired

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SweepOptions are the choices one sweep makes about the tree it reads.
type SweepOptions struct {
	// Exclude names repo-relative paths the sweep does not read. The file
	// that PUBLISHES the names is one of them, and so is a records table:
	// both are places where the names are SUPPOSED to appear.
	Exclude []string
}

// Coverage is what one sweep read, and what it did not.
//
// WHY THE REACH IS REPORTED. A sweep that names no coverage is a claim
// nobody can re-derive, and the org's own rule is that a control states what
// it covered — so a reader never has to guess how far a verdict reached, and
// a file that was skipped for a reason is visible instead of silently absent.
type Coverage struct {
	// Files is the number of files the sweep read.
	Files int
	// GoFiles is the number of those that are Go sources.
	GoFiles int
	// TextFiles is the number of those that are text surfaces.
	TextFiles int
	// SkippedTests counts the test files, which hold the assertions OF
	// ABSENCE — the name is there on purpose, and its presence is the proof.
	SkippedTests int
	// SkippedHistory counts the files under a CHANGELOG — the record of
	// what happened, not a surface a consumer reads.
	SkippedHistory int
	// SkippedBinary counts the files that are not text.
	SkippedBinary int
	// SkippedNested counts the nested repositories — a submodule is a repo
	// of its own, with its own rulebook and its own gates.
	SkippedNested int
	// SkippedSymlinks counts the SYMLINKS — a link is another path's
	// content, not this tree's.
	SkippedSymlinks int
	// SkippedDirs counts the dependency directories (vendor, node_modules,
	// testdata, .git) the sweep does not descend into.
	SkippedDirs int
	// Excluded counts the paths the caller excluded by name.
	Excluded int
}

// SweepResult is what a sweep found, and how far it reached.
type SweepResult struct {
	// Findings are the hits: every occurrence of a withdrawn name the
	// sweep read.
	Findings []Finding
	// Coverage is what the sweep read and what it skipped.
	Coverage Coverage
}

// SweepTree reports every withdrawn identifier the tree under root carries,
// over EVERY surface the tree has.
//
// WHY A SWEEP AND NOT THE CONTRACT'S OWN SCAN. The contract's guard
// (ScanTree) reads the contract's OWN surfaces — Go sources, CUE schemas,
// protobuf schemas — because that is where the contract's names live. The
// org's trees have more surfaces than that: an authored yaml, a skill's
// prose, a script, a check bed. A withdrawn name in those is just as live,
// and a scan that reads only three extensions cannot see it. So the sweep
// reads EVERY text file, under the same rules the contract's own scan
// applies to the surfaces it already reads:
//
//   - a Go source is read STRUCTURALLY — declarations and literals, never
//     comments, because a comment naming a withdrawn identifier is the
//     RECORD of the withdrawal, and a declaration is a surface a consumer
//     can still reach;
//   - every other text surface is read TEXTUALLY, comments included,
//     because the prose IS the published surface;
//   - a SCOPED name is never swept on a text surface: a scope is a Go type,
//     and a text file has no such notion;
//   - a TEST file holds the assertions OF ABSENCE and is skipped;
//   - a CHANGELOG is history, not a surface;
//   - a nested repository is a repo of its own;
//   - a SYMLINK is another path's content, not this tree's;
//   - a file that is not text is not a surface.
//
// The contract's own guard keeps reading the contract's surfaces with
// ScanTree; this sweep is the ORG-WIDE reach, and the two share one set of
// rules and one token matcher.
func SweepTree(root string, ids []Identifier, opts SweepOptions) (SweepResult, error) {
	excluded := make(map[string]bool, len(opts.Exclude))
	for _, path := range opts.Exclude {
		excluded[filepath.ToSlash(filepath.Clean(path))] = true
	}

	var result SweepResult
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if entry.IsDir() {
			switch {
			case skipDir(entry.Name()):
				result.Coverage.SkippedDirs++
				return filepath.SkipDir
			case isHistoryPath(rel):
				result.Coverage.SkippedHistory++
				return filepath.SkipDir
			case rel != "." && isNestedRepo(path):
				// The ROOT is never a nested repo: a checkout that is a
				// WORKTREE carries a .git FILE at its root, and skipping
				// that tree would sweep nothing and report a clean tree.
				result.Coverage.SkippedNested++
				return filepath.SkipDir
			}
			return nil
		}

		if entry.Type()&fs.ModeSymlink != 0 {
			// A SYMLINK is another PATH's content, not this tree's. The
			// umbrella's own walk states the rule once: the harness farm
			// (.dsh/skills/*) points into the corpus, which is another
			// repo, and reading through one judges that repo's content
			// under this repo's rulebook.
			//
			// It is also why a DANGLING link must not fail the run: a
			// link whose target is not materialized reads as ENOENT, and
			// one broken link in one repo would fail the arm for every
			// repo.
			result.Coverage.SkippedSymlinks++
			return nil
		}

		if entry.Name() == ".git" {
			// The gitlink of a worktree or a submodule checkout: the
			// repository's own metadata, not a surface.
			result.Coverage.Excluded++
			return nil
		}

		if excluded[rel] {
			result.Coverage.Excluded++
			return nil
		}
		if isHistoryPath(rel) {
			result.Coverage.SkippedHistory++
			return nil
		}

		var found []Finding
		switch strings.ToLower(filepath.Ext(path)) {
		case ".go":
			if strings.HasSuffix(path, "_test.go") {
				result.Coverage.SkippedTests++
				return nil
			}
			scanned, scanErr := scanGoFile(path, rel, ids)
			if scanErr != nil {
				return scanErr
			}
			found = scanned
			result.Coverage.GoFiles++
		case ".cue", ".proto":
			scanned, scanErr := scanTextFile(path, rel, ids)
			if scanErr != nil {
				return scanErr
			}
			found = scanned
			result.Coverage.TextFiles++
		default:
			text, isText, readErr := readTextFile(path)
			if readErr != nil {
				return readErr
			}
			if !isText {
				result.Coverage.SkippedBinary++
				return nil
			}
			found = scanText(text, rel, ids)
			result.Coverage.TextFiles++
		}
		result.Coverage.Files++
		result.Findings = append(result.Findings, found...)
		return nil
	})
	if err != nil {
		return SweepResult{}, err
	}
	return result, nil
}

// isHistoryPath reports whether a path is HISTORY: a file under a CHANGELOG
// directory, or a CHANGELOG file beside it.
//
// WHY HISTORY IS NOT A SURFACE. The org's rule is that history lives only in
// CHANGELOG/, and a CHANGELOG entry that names a withdrawn identifier is the
// RECORD of the withdrawal — it is what happened, written down. A sweep that
// read history would report every withdrawal the org has ever made, which is
// the opposite of a finding.
func isHistoryPath(rel string) bool {
	if strings.HasPrefix(rel, "CHANGELOG/") {
		return true
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "CHANGELOG" {
			return true
		}
	}
	return filepath.Base(rel) == "CHANGELOG.md"
}

// isNestedRepo reports whether a directory is a NESTED repository — a
// submodule, or a checkout inside the tree.
//
// WHY A NESTED REPO IS SKIPPED. A submodule is a repo of its own: it has its
// own rulebook, its own gates, and its own history. Its files are the
// umbrella's PIN, not the umbrella's content, and judging them under this
// repo's rulebook is how one copy of a walk judges another repo's content.
func isNestedRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && !info.IsDir()
}

// readTextFile reads a file that is TEXT, and reports whether it is.
//
// A file that carries a NUL byte in its first block is not text: it is a
// binary artifact, and a withdrawn name inside it is not a surface a
// consumer reads.
func readTextFile(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 8192)
	read, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", false, fmt.Errorf("reading %s: %w", path, err)
	}
	if bytes.IndexByte(head[:read], 0) >= 0 {
		return "", false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), true, nil
}
