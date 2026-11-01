package retired

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Record is the ORG's classification of one hit: the hit is a RECORD — a
// mention that is correct — and not a CARRIER, a live use of a withdrawn
// name.
//
// WHY A HIT IS CLASSIFIED AND NOT ASSUMED. The org's own sweeps classify
// every hit BY HAND: history stays; a counter-instance that DENIES the
// withdrawn name stays; an explanatory comment that names it stays; a live
// claim is a carrier. The distinction is semantic — a sentence that denies a
// withdrawn name and a sentence that sets its value carry the same name and
// mean opposite things — so a control that reported both as defects would
// cry wolf on the org's own correct documentation, and a control that GUESSED
// the distinction would miss the carriers it exists to catch. So the
// classification is COMMITTED, and every record says WHY.
//
// WHY THE RECORDS LIVE OUTSIDE THE CONTRACT. A record names a file in a
// repository the contract does not own, and the contract's own tree has no
// records at all — its guard reads its surfaces directly. The table is the
// ORG's, and it is DATA the contract's rule reads; the rule for what makes
// a hit a carrier lives here, once.
type Record struct {
	// Repo is the repository the record is about. A record applies to the
	// tree whose repository it names, and to no other: a table that covers
	// the org must not make one repo's record another repo's excuse.
	Repo string `json:"repo"`
	// File is the repository-relative path of the hit the record covers.
	File string `json:"file"`
	// Identifier is the withdrawn NAME the record covers. A record covers
	// only the identifier it names, so a record written about one name can
	// never excuse another.
	Identifier string `json:"identifier"`
	// Contains is the text the covered hit's LINE must carry. Empty means
	// the record covers every hit of that identifier in that file.
	Contains string `json:"contains,omitempty"`
	// Why says why the mention is a record rather than a use.
	Why string `json:"why"`
}

// RecordsFile is the shape of a records table.
type RecordsFile struct {
	Records []Record `json:"records"`
}

// LoadRecords reads a records table from disk.
func LoadRecords(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the records %s: %w", path, err)
	}
	var file RecordsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decoding the records %s: %w", path, err)
	}
	return file.Records, nil
}

// ValidateRecords checks a table against the registry.
//
// A record may not INVENT a name: the withdrawn names are the contract's,
// and a record about a name the contract does not publish is a record about
// nothing. A record must also say WHY — a record without a reason is an
// excuse with no argument, and it is what a table rots into.
func ValidateRecords(records []Record, ids []Identifier) error {
	published := map[string]bool{}
	for _, id := range ids {
		published[id.Name] = true
	}
	for _, record := range records {
		if record.File == "" {
			return fmt.Errorf("the record %+v names no file", record)
		}
		if record.Repo == "" {
			return fmt.Errorf("the record about %s names no repository", record.File)
		}
		if !published[record.Identifier] {
			return fmt.Errorf("the record about %s names the identifier %q, which the contract does not publish", record.File, record.Identifier)
		}
		if record.Why == "" {
			return fmt.Errorf("the record about %s says nothing about why the mention is a record", record.File)
		}
	}
	return nil
}

// Match is one hit a record covers.
type Match struct {
	// Record is the record that covers the hit.
	Record Record
	// Finding is the hit it covers.
	Finding Finding
}

// Classification is the verdict over a set of hits.
type Classification struct {
	// Carriers are the hits no record covers: a live use of a withdrawn
	// name, and the thing the org's arm exists to catch.
	Carriers []Finding
	// Matches are the hits a record covers.
	Matches []Match
	// Stale are the records that cover no hit. A record that covers
	// nothing is ITSELF a finding: a table that rots into a hole hides a
	// live use behind a record that no longer says anything.
	Stale []Record
}

// Classify judges the hits against the records.
//
// A record covers a hit when it names the swept repository, the hit's file,
// and the hit's identifier, and when the hit's line carries the record's
// text. Every record that applies to the swept repository and covers no hit
// is stale; the records of OTHER repositories are not judged here, because
// they are about other trees.
func Classify(root string, findings []Finding, records []Record, repo string) (Classification, error) {
	var verdict Classification
	covered := make([]bool, len(records))
	for _, finding := range findings {
		text, err := lineText(root, finding.File, finding.Line)
		if err != nil {
			return Classification{}, err
		}
		matched := false
		for i, record := range records {
			if !recordCovers(record, finding, text, repo) {
				continue
			}
			verdict.Matches = append(verdict.Matches, Match{Record: record, Finding: finding})
			covered[i] = true
			matched = true
		}
		if !matched {
			verdict.Carriers = append(verdict.Carriers, finding)
		}
	}
	for i, record := range records {
		if record.Repo != repo {
			continue
		}
		if !covered[i] {
			verdict.Stale = append(verdict.Stale, record)
		}
	}
	return verdict, nil
}

// recordCovers reports whether a record covers a hit.
func recordCovers(record Record, finding Finding, text, repo string) bool {
	if record.Repo != repo {
		return false
	}
	if record.File != finding.File {
		return false
	}
	if record.Identifier != finding.Identifier.Name {
		return false
	}
	if record.Contains != "" && !strings.Contains(text, record.Contains) {
		return false
	}
	return true
}

// lineText returns the text of a file's line, or the empty string when the
// file no longer has that line.
func lineText(root, rel string, line int) (string, error) {
	if line < 1 {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", rel, err)
	}
	lines := strings.Split(string(data), "\n")
	if line > len(lines) {
		return "", nil
	}
	return lines[line-1], nil
}
