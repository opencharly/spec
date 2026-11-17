package schemaparams

// pattern_map_test.go — the R7 half of opencharly/spec#271.
//
// `emitStructKind` asked `val.LookupPath(cue.MakePath(cue.AnyString))` whether a struct is a
// map. `cue.AnyString` is the UNCONSTRAINED label `_`, so that lookup selects only the ellipsis
// `[string]: T`; a universal pattern constraint -- `[#Word]: T`, or `[("a" | "b")]: T` -- does
// not resolve through it, fell through to the struct branch, and closed immediately as
// `struct {}`, because a pattern contributes no FIELD. The payload was not lost at that point;
// it was never asked for. `spec.CandyUse.Secret` is where that showed: the delivery contract
// carried a `map` of `{src, dst, optional}` in CUE and an empty struct in Go, so no caller
// could read which file to deliver (#271, and P07 unit 2b waiting on it).
//
// THE ORACLE IS NOT WEAKENED HERE. `TestGenerateNoImportIsByteIdenticalToGengotypes` pins this
// generator to the pinned `cue exp gengotypes`, and its fixture (`testdata/twin/schema/`,
// one file) contains no pattern-keyed collection -- so it makes no claim about this shape and
// it stays byte-for-byte true. What this file adds is a test OF A CASE THE ORACLE NEVER
// COVERED, plus a measurement of the divergence so it cannot silently drift:
//
//	$ cue v0.16.1 exp gengotypes .        # on the fixture below
//	type Inline struct {
//	}                                     <-- the payload is gone
//
// The branch that does that lives upstream in
// cuelang.org/go@v0.16.1/internal/encoding/gotypes/generate.go:387, under `internal/`: no
// external module can import it, let alone patch it. That is why the divergence is OURS to
// own, and why deleting this branch to "match the oracle" would restore the defect.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// patternFixture exercises the shapes the fix must tell apart, plus the guard.
//
// The last two cases carry a REQUIRED field in the element -- the shape that exposed the second
// half of this defect. A closed definition merged with the lease fields evaluates to BOTTOM
// ("exclusive: field not allowed"), so the emitter reached its `any /* TODO */` fallback and the
// payload was lost again, spelled `map[string]any` instead of `struct {}`. The schema no longer
// produces that bottom (schema/capability.cue declares the lease field on the leasable shapes);
// these cases pin the generator side: whatever the element is, a required field must survive
// into the map's value type.
const patternFixture = `package schema

#Word: =~"^[a-z][a-z0-9_]*$"
#Entry: {count: int, label?: string}

#Inline: {[#Word]: {count: int}}
#Refd: {[#Word]: #Entry}
#Disj: {[("nvidia" | "amd" | "intel")]: {n: int}}
#Ellip: {[string]: int}
#Mixed: {[=~"^zz"]: int, named: string}

// a REQUIRED field in the element (see the Go comment above)
#Strict: {count!: int}
#ReqRef: {[#Word]: #Strict}
#ReqInline: {[#Word]: {count!: int}}
`

// writePatternFixture materializes the fixture in a temp schema dir.
func writePatternFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.cue"), []byte(patternFixture), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	return dir
}

// TestPatternKeyedCollectionIsAMapCarryingItsPayload is the R7 proof. Both halves matter: the
// CONTAINER must change (`map[string]`, not `struct {}`), and the PAYLOAD must arrive with its
// value type -- a test that only greps for `map[` would pass on a map of nothing, which is the
// same defect wearing a different word.
func TestPatternKeyedCollectionIsAMapCarryingItsPayload(t *testing.T) {
	out, err := Generate(writePatternFixture(t), "params")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src := string(out)

	t.Run("a regex-keyed collection is a map carrying its payload", func(t *testing.T) {
		if !strings.Contains(src, "type Inline map[string]struct {") {
			t.Fatalf("`#Inline: {[#Word]: {count: int}}` did not generate a map:\n%s", src)
		}
		// THE PAYLOAD. `count: int` must survive as the element's own field, with its Go type.
		block := blockFor(t, src, "Inline")
		if !strings.Contains(block, "Count int64") {
			t.Fatalf("the element type lost `count` -- a map of nothing is the same defect:\n%s", block)
		}
	})

	t.Run("a definition-reference value keeps its named Go type", func(t *testing.T) {
		if !strings.Contains(src, "type Refd map[string]Entry") {
			t.Fatalf("`#Refd: {[#Word]: #Entry}` did not generate `map[string]Entry`:\n%s", src)
		}
		if !strings.Contains(src, "type Entry struct {") {
			t.Fatalf("the referenced element type was not generated:\n%s", src)
		}
	})

	t.Run("a literal-disjunction key is a string-keyed map", func(t *testing.T) {
		block := blockFor(t, src, "Disj")
		if !strings.HasPrefix(block, "type Disj map[string]struct {") {
			t.Fatalf("`#Disj: {[(\"nvidia\" | \"amd\" | \"intel\")]: {n: int}}` is not a string-keyed map:\n%s", block)
		}
		// Go has no sum type for a key, so the word set is closed by the word type's Valid(),
		// never by the Go key type; the payload must still be there.
		if !strings.Contains(block, "N int64") {
			t.Fatalf("the disjunction-keyed element lost `n`:\n%s", block)
		}
	})

	t.Run("the ellipsis stays byte-identical to what it always was", func(t *testing.T) {
		if !strings.Contains(src, "type Ellip map[string]int64") {
			t.Fatalf("`#Ellip: {[string]: int}` regressed (it must stay `map[string]int64`):\n%s", src)
		}
	})

	t.Run("an element with a REQUIRED field keeps its payload", func(t *testing.T) {
		block := blockFor(t, src, "ReqInline")
		if !strings.HasPrefix(block, "type ReqInline map[string]struct {") {
			t.Fatalf("`#ReqInline: {[#Word]: {count!: int}}` is not a map:\n%s", block)
		}
		if !strings.Contains(block, "Count int64") {
			t.Fatalf("the required field did not survive into the value type:\n%s", block)
		}
		// The definition-reference form must keep its NAME, not an anonymous struct.
		ref := blockFor(t, src, "ReqRef")
		if !strings.HasPrefix(ref, "type ReqRef map[string]Strict") {
			t.Fatalf("`#ReqRef: {[#Word]: #Strict}` did not keep the named element type:\n%s", ref)
		}
		if !strings.Contains(blockFor(t, src, "Strict"), "Count int64") {
			t.Fatalf("the referenced required-field type lost its payload:\n%s", blockFor(t, src, "Strict"))
		}
	})

	t.Run("a struct mixing a pattern with named fields is NOT flattened to a map", func(t *testing.T) {
		block := blockFor(t, src, "Mixed")
		if strings.HasPrefix(block, "type Mixed map[") {
			t.Fatalf("a mixed struct was flattened to a map, which drops the named field:\n%s", block)
		}
		if !strings.Contains(block, "Named string") {
			t.Fatalf("the named field was lost:\n%s", block)
		}
	})
}

// TestThePinnedCLIStillCollapsesPatternMaps measures the divergence this fix owns, so it can
// never drift into an unstated one. It runs the PINNED CLI on the same fixture: if upstream
// ever emits a map, this test fails and the documented divergence can be retired. It skips
// visibly when the pinned binary cannot be provisioned (live or skip, never fake).
func TestThePinnedCLIStillCollapsesPatternMaps(t *testing.T) {
	cue := pinnedCue(t)
	if cue == "" {
		t.Skip("the pinned cue binary could not be provisioned, so the divergence cannot be measured here; " +
			"R8: live or skip, never fake. The oracle test skips the same way.")
	}
	dir := writePatternFixture(t)
	out := string(gengotypesOracle(t, dir, "params", cue))
	if !strings.Contains(out, "type Inline struct {") {
		t.Fatalf("the pinned `cue exp gengotypes` NO LONGER collapses a pattern-keyed collection — the divergence this "+
			"file documents is gone, so retire it (and re-check whether the payload survives):\n%s", out)
	}
	if !strings.Contains(out, "type Ellip map[string]int64") {
		t.Fatalf("the pinned CLI disagrees on the ellipsis case too, which would mean a different divergence than the one documented:\n%s", out)
	}
}

// blockFor returns the generated block that starts with `type <name> ` up to the closing brace
// at column zero, so a payload assertion cannot accidentally match a neighbouring type.
func blockFor(t *testing.T, src, name string) string {
	t.Helper()
	start := strings.Index(src, "type "+name+" ")
	if start < 0 {
		t.Fatalf("no `type %s` in the generated output:\n%s", name, src)
	}
	rest := src[start:]
	if end := strings.Index(rest, "\n}\n"); end >= 0 {
		return rest[:end+2]
	}
	return rest
}
