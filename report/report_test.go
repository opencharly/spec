package report

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/opencharly/spec/spec"
)

// resultsWithNewlines is a report whose steps carry the shape that broke every reader of
// the text format: a plan step whose `run:` block is a multi-line script, and a result
// message that quotes a multi-line output. It is the shape opencharly/layer-nodejs's
// candy contributes to every bed that builds it (opencharly/spec#282).
//
// It is the FIXTURE, not the proof: the tests below assert the SHAPE of the rendered
// report, so a renderer that writes a step's text verbatim fails them.
func resultsWithNewlines() []spec.StepResult {
	return []spec.StepResult{
		{
			Keyword: "run",
			Text: "command=case \"${BUILD_ARCH}\" in\n" +
				"  x86_64)  PNPM_ARCH=\"x64\" ;;\n" +
				"  aarch64) PNPM_ARCH=\"arm64\" ;;\n" +
				"esac\n" +
				"curl -fsSL \"https://example.invalid/pnpm\" -o /usr/local/bin/pnpm",
			Origin: "plan:candy:github.com/opencharly/layer-nodejs",
			StepID: "plan:candy:github.com/opencharly/layer-nodejs:0",
			Result: spec.CheckResult{
				Status:  spec.StatusSkip,
				Message: "skipped — verify-only mode (mutating step)",
			},
		},
		{
			Keyword: "check",
			Text:    "the site builds",
			Origin:  "docs-site-app",
			StepID:  "docs-site-index-built",
			Result: spec.CheckResult{
				Status: spec.StatusFail,
				Message: "npm ci failed\n" +
					"npm ERR! code EUSAGE\n" +
					"npm ERR! A complete log of this run can be found in: /root/.npm/_logs/1.log",
			},
		},
	}
}

// stepStatusWord is the status word a step's line carries. It is the step's own status,
// except for the INFRA rewrite — a container-SETUP failure is labelled INFRA so it reads
// apart from a real FAIL.
func stepStatusWord(step spec.StepResult) string {
	return strings.ToUpper(step.Result.Status.String())
}

// TestFormatStepResultsText_OneLinePerStep is the load-bearing test: the text report is a
// LINE format, and a reader pairs each step's status with its id by parsing one line per
// step. A step that SPANS lines carries its id bracket on a line of its own, and the
// report's step lines then disagree with the report's own rollup — so a reader that checks
// the two refuses the whole report and credits nothing from it.
//
// The four assertions are the format's contract: one line per step, the id on exactly one
// line, the STATUS on that same line, and the rollup counting the lines the report
// carries.
func TestFormatStepResultsText_OneLinePerStep(t *testing.T) {
	results := resultsWithNewlines()
	var buf bytes.Buffer
	FormatStepResultsText(&buf, results)
	out := buf.String()

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	// One line per step, then a blank line, then the rollup.
	if len(lines) != len(results)+2 {
		t.Fatalf("the report carries %d step line(s) for %d step(s); a step must be ONE line:\n%s",
			len(lines)-2, len(results), out)
	}
	if lines[len(lines)-2] != "" {
		t.Fatalf("the line before the rollup is not blank: %q", lines[len(lines)-2])
	}
	rollup := lines[len(lines)-1]
	if want := fmt.Sprintf("%d steps:", len(results)); !strings.Contains(rollup, want) {
		t.Fatalf("the rollup does not count the steps (%q missing from %q)", want, rollup)
	}

	statusLines := 0
	for _, line := range lines {
		if isStatusLine(line) {
			statusLines++
		}
	}
	if statusLines != len(results) {
		t.Fatalf("the report carries %d status line(s) for %d step(s):\n%s", statusLines, len(results), out)
	}

	for i, step := range results {
		want := "[" + step.StepID + "]"
		found := -1
		for j, line := range lines {
			if !strings.Contains(line, want) {
				continue
			}
			if found >= 0 {
				t.Fatalf("step %d's id appears on more than one line; a step is ONE line:\n%s", i, out)
			}
			found = j
		}
		if found < 0 {
			t.Fatalf("step %d's id appears on no line:\n%s", i, out)
		}
		// The line that carries the id must carry the STATUS too — those two are what a
		// reader pairs, and a reader that cannot pair them credits nothing.
		if !isStatusLine(lines[found]) {
			t.Fatalf("step %d's id is on a line that does not carry a status: %q", i, lines[found])
		}
		if !strings.HasPrefix(lines[found], "  "+stepStatusWord(step)) {
			t.Fatalf("step %d's line carries the wrong status: %q", i, lines[found])
		}
		// The id is separated from the text by two spaces, and a flattened text carries no
		// run of two spaces — so the separator is the only one on the line and a bracket
		// inside the text cannot steal the split.
		if n := strings.Count(lines[found], "  ["); n != 1 {
			t.Fatalf("step %d's line carries %d id separator(s); the id must be unambiguous: %q", i, n, lines[found])
		}
	}

	// The FAILURE's multi-line message is flattened onto its step's line, not spilled over
	// the lines after it.
	if want := "npm ERR! A complete log of this run can be found in: /root/.npm/_logs/1.log"; !strings.Contains(lines[1], want) {
		t.Fatalf("the failure message was not carried on its step's line: %q", lines[1])
	}
}

// isStatusLine reports whether a line carries a step's STATUS — the word a reader reads
// the verdict from.
func isStatusLine(line string) bool {
	for _, word := range []string{"PASS", "FAIL", "SKIP", "INFRA"} {
		if strings.HasPrefix(line, "  "+word+" ") {
			return true
		}
	}
	return false
}

// tapConstruct matches every line a TAP report may carry: the header, the plan, a test
// point, or a field of a point's diagnostic block. A line that matches NONE of them is a
// step that spilled over its own point.
var tapConstruct = regexp.MustCompile(`^(TAP version |1\.\.|ok |not ok |  ---$|  \.\.\.$|  [a-z_]+: )`)

// TestFormatStepResultsTAP_OnePointPerStep asserts the same shape for TAP: a test point
// is one line, so a step whose text carries a newline must not split its point.
func TestFormatStepResultsTAP_OnePointPerStep(t *testing.T) {
	results := resultsWithNewlines()
	var buf bytes.Buffer
	FormatStepResultsTAP(&buf, results)
	out := buf.String()

	points := map[int]string{}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if !tapConstruct.MatchString(line) {
			t.Fatalf("the TAP report carries a line that is not a TAP construct — a step spilled over its own point:\n%s", out)
		}
		body := strings.TrimPrefix(line, "not ok ")
		body = strings.TrimPrefix(body, "ok ")
		if body == line {
			continue
		}
		num := body
		if i := strings.IndexByte(body, ' '); i >= 0 {
			num = body[:i]
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			t.Fatalf("the TAP report carries a test point with no number: %q", line)
		}
		points[n] = line
	}

	for i, step := range results {
		line, found := points[i+1]
		if !found {
			t.Fatalf("the TAP report carries no test point for step %d:\n%s", i+1, out)
		}
		// The point carries the step's WHOLE text — the whole text, flattened.
		if !strings.Contains(line, OneLine(step.Text)) {
			t.Fatalf("test point %d does not carry the step's whole text: %q", i+1, line)
		}
	}
}

// TestTruncate_CutsOnARuneBoundary locks the truncation to a CHARACTER boundary. A byte
// cut can land inside a multi-byte rune and emit invalid UTF-8 into a report whose whole
// value is that a reader can read it.
func TestTruncate_CutsOnARuneBoundary(t *testing.T) {
	// "→" is three bytes, so a byte cut at 100 lands inside the 34th rune.
	s := strings.Repeat("→", 200)
	got := truncate(s, 100)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 101 {
		t.Fatalf("truncate produced %d rune(s), want 101 (100 + the ellipsis)", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate did not mark the cut: %q", got)
	}
}

// TestOneLine_FlattensEveryWhitespaceRun locks the ONE flattening rule the report uses.
func TestOneLine_FlattensEveryWhitespaceRun(t *testing.T) {
	got := OneLine("command=case \"${BUILD_ARCH}\" in\n  x86_64)  PNPM_ARCH=\"x64\" ;;\nesac")
	want := `command=case "${BUILD_ARCH}" in x86_64) PNPM_ARCH="x64" ;; esac`
	if got != want {
		t.Fatalf("OneLine = %q, want %q", got, want)
	}
	if strings.Contains(got, "  ") {
		t.Fatalf("OneLine left a run of two spaces: %q", got)
	}
}
