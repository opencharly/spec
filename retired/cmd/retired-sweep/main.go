// retired-sweep — assert that a tree carries no withdrawn identifier the
// contract has not classified as a RECORD.
//
// WHY A COMMAND. The org's arm runs this over EVERY repository's tree, from
// a checkout of the contract at a PINNED tag, so the names it judges are the
// names that tag publishes — the EMBEDDED registry, never a list passed in.
// A --list flag would be a second way to say which names are withdrawn, and
// two ways is how a name drifts.
//
// WHY THE RECORDS ARE A TABLE AND NOT A RULE. A hit is not automatically a
// carrier: the org's own sweeps classify every hit by hand — history stays;
// a counter-instance that DENIES the withdrawn name stays; an explanatory
// comment that names it stays; a live claim is a carrier. The table says
// which is which, and every entry says why. A hit no entry covers is a
// carrier; an entry that covers no hit is ITSELF a finding, so the table
// cannot rot into a hole that hides a live use.
//
// usage: retired-sweep --tree <dir> [--repo <slug>] [--records <file>] [--exclude <path>]... [--json]
//
// exit: 0 the tree is clean · 1 a carrier or a stale record · 2 usage.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencharly/spec/retired"
)

// listPath is where the contract publishes the names, relative to the
// contract's module root. It is EXCLUDED from every sweep: it is where the
// names are supposed to appear.
const listPath = "retired/identifiers.json"

// report is the machine-readable form of the verdict.
type report struct {
	Repo     string           `json:"repo"`
	Tree     string           `json:"tree"`
	Coverage retired.Coverage `json:"coverage"`
	Carriers []carrier        `json:"carriers"`
	Records  []matched        `json:"records"`
	Stale    []retired.Record `json:"stale"`
}

// carrier is one hit no record covers.
type carrier struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Ident  string `json:"identifier"`
	Detail string `json:"detail"`
}

// matched is one hit a record covers.
type matched struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Ident string `json:"identifier"`
	Why   string `json:"why"`
	Has   bool   `json:"has_contains"`
}

func main() {
	tree := flag.String("tree", "", "the tree to sweep (required)")
	repo := flag.String("repo", "", "the repository the tree belongs to; the records are matched by it")
	records := flag.String("records", "", "the records table to judge the hits against")
	asJSON := flag.Bool("json", false, "print the verdict as JSON")
	var excludes multiFlag
	flag.Var(&excludes, "exclude", "a repo-relative path the sweep does not read (repeatable)")
	flag.Usage = usage
	flag.Parse()

	if *tree == "" {
		fmt.Fprintln(os.Stderr, "retired-sweep: --tree is required")
		usage()
		os.Exit(2)
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "retired-sweep: unexpected argument %q\n", flag.Arg(0))
		usage()
		os.Exit(2)
	}

	root, err := filepath.Abs(*tree)
	if err != nil {
		fatal("resolving the tree: %v", err)
	}

	excluded := append([]string{listPath}, excludes...)
	if *records != "" {
		recordsPath, absErr := filepath.Abs(*records)
		if absErr != nil {
			fatal("resolving the records: %v", absErr)
		}
		// A records table that lives INSIDE the swept tree is excluded
		// from it: it is where the names are supposed to appear, exactly
		// like the list the contract publishes.
		if rel, relErr := filepath.Rel(root, recordsPath); relErr == nil && !strings.HasPrefix(rel, "..") {
			excluded = append(excluded, filepath.ToSlash(rel))
		}
	}

	result, err := retired.SweepTree(root, retired.Identifiers, retired.SweepOptions{Exclude: excluded})
	if err != nil {
		fatal("sweeping the tree: %v", err)
	}

	var table []retired.Record
	if *records != "" {
		table, err = retired.LoadRecords(*records)
		if err != nil {
			fatal("%v", err)
		}
		if err := retired.ValidateRecords(table, retired.Identifiers); err != nil {
			fatal("the records table: %v", err)
		}
	}

	verdict, err := retired.Classify(root, result.Findings, table, *repo)
	if err != nil {
		fatal("classifying the hits: %v", err)
	}

	if *asJSON {
		out := report{Repo: *repo, Tree: root, Coverage: result.Coverage}
		for _, finding := range verdict.Carriers {
			out.Carriers = append(out.Carriers, carrier{
				File: finding.File, Line: finding.Line, Ident: finding.Identifier.Name, Detail: finding.Detail,
			})
		}
		for _, match := range verdict.Matches {
			out.Records = append(out.Records, matched{
				File: match.Finding.File, Line: match.Finding.Line, Ident: match.Finding.Identifier.Name,
				Why: match.Record.Why, Has: match.Record.Contains != "",
			})
		}
		out.Stale = verdict.Stale
		encoded, encodeErr := json.MarshalIndent(out, "", "  ")
		if encodeErr != nil {
			fatal("encoding the verdict: %v", encodeErr)
		}
		fmt.Println(string(encoded))
	} else {
		printVerdict(*repo, result.Coverage, verdict)
	}

	if len(verdict.Carriers) > 0 || len(verdict.Stale) > 0 {
		os.Exit(1)
	}
}

// printVerdict prints the coverage, the hits, and the verdict — the reach
// first, because a verdict whose reach is unstated is a claim nobody can
// re-derive.
func printVerdict(repo string, coverage retired.Coverage, verdict retired.Classification) {
	fmt.Printf("retired-sweep: %s: %d file(s) read (%d go, %d text); skipped %d test file(s), %d history file(s), %d binary file(s), %d nested repo(s), %d symlink(s), %d dependency dir(s), %d excluded path(s)\n",
		repo, coverage.Files, coverage.GoFiles, coverage.TextFiles,
		coverage.SkippedTests, coverage.SkippedHistory, coverage.SkippedBinary,
		coverage.SkippedNested, coverage.SkippedSymlinks, coverage.SkippedDirs, coverage.Excluded)
	for _, finding := range verdict.Carriers {
		fmt.Printf("CARRIER %s\n", finding)
	}
	for _, match := range verdict.Matches {
		fmt.Printf("RECORD  %s:%d: the withdrawn %s %s is present (%s) — %s\n",
			match.Finding.File, match.Finding.Line, match.Finding.Identifier.Kind, match.Finding.Identifier,
			match.Finding.Detail, match.Record.Why)
	}
	for _, record := range verdict.Stale {
		fmt.Printf("STALE   %s:%s: the withdrawn %s — the record covers no hit\n",
			record.Repo, record.File, record.Identifier)
	}
	switch {
	case len(verdict.Carriers) > 0 && len(verdict.Stale) > 0:
		fmt.Printf("FAIL    %d carrier(s) and %d stale record(s)\n", len(verdict.Carriers), len(verdict.Stale))
	case len(verdict.Carriers) > 0:
		fmt.Printf("FAIL    %d carrier(s): a withdrawn name is live in the tree\n", len(verdict.Carriers))
	case len(verdict.Stale) > 0:
		fmt.Printf("FAIL    %d stale record(s): a record covers no hit\n", len(verdict.Stale))
	default:
		fmt.Println("PASS    no withdrawn name is live in the tree")
	}
}

// multiFlag is a flag that may be repeated.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: retired-sweep --tree <dir> [--repo <slug>] [--records <file>] [--exclude <path>]... [--json]

  --tree      the tree to sweep (required)
  --repo      the repository the tree belongs to; the records are matched by it
  --records   the records table to judge the hits against
  --exclude   a repo-relative path the sweep does not read (repeatable)
  --json      print the verdict as JSON
`)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "retired-sweep: "+format+"\n", args...)
	os.Exit(2)
}
