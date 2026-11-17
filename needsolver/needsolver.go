// Package needsolver is the ONE solver D-CAP-3 requires: every `need:` and `use:` in a deployment's
// stack is bound by the single provide of its capability it can see, or the load fails naming what
// was searched.
//
// It lives in `spec` for the same reason `storeroot` does: it is one implementation of a
// cross-cutting concern that the HOST must perform (D-CAP-3's "one solver"), and `charly/` core may
// import `spec/*` only — never an sdk mechanism kit (IMPORT-PURITY). `sdk` re-exports it exactly as
// `sdk/storekit` re-exports `spec/storeroot`.
//
// What this package deliberately does NOT do:
//   - it does not deliver a binding. D-CAP-4 says bindings are STATE, delivered by the type owner's
//     `bind`; this solver only decides WHICH provide a need is bound to.
//   - it does not read files or the registry. Visibility (§8.2) is the caller's to gather — the
//     stack, the containing nodes, the alongside nodes, the registry, the grant — because only the
//     caller knows the tree. This package resolves the set it is HANDED.
package needsolver

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Strength is a capability's strength, which D-CAP-1 makes THE KEY rather than a field: a `need:`
// must be bound and fails when it is not; a `use:` is bound when a provide exists and takes its
// default, or goes without.
type Strength int

const (
	// Need is a `need:` entry — must be bound, load fails otherwise (D-CAP-1).
	Need Strength = iota
	// Use is a `use:` entry — may take its `default:` instead of a provide (D-CAP-1).
	Use
)

// Capability names what is wanted: a type plus the name under it. `secret`/`endpoint` are host
// capability types (D-CAP-2); `role` and `api` are the registry's (D-CAP-3 visibility 4).
type Capability struct {
	Type string
	Name string
}

// String is the contract's own spelling, so an error message names what an author wrote.
func (c Capability) String() string { return c.Type + ":" + c.Name }

// Want is one `need:` or `use:` entry, and where it was authored (for the error).
type Want struct {
	Capability
	Strength Strength
	// Default is a `use:` entry's `default:`; unused for a Need. Its PRESENCE (not its value) is
	// what D-CAP-1 lets a use fall back to, so it is a pointer.
	Default *string
	// Authored is where this entry was written ("candy:foo", "deploy:bar"), used verbatim in errors
	// so a failure tells the author where to look.
	Authored string
}

// Provide is one `provide` entry and the stack member that offers it — "the one provide ... it can
// see" (§8.2). Origin is named in errors, because two visible provides of one capability are an
// error that must name BOTH.
type Provide struct {
	Capability
	Origin string
}

// Binding is the solver's answer for one Want: the provide it is bound to, or a default.
type Binding struct {
	Want
	// Bound is the provide's Origin; empty when the entry took its Default.
	Bound string
}

// TookDefault reports whether this binding fell back to a `use:` default rather than a provide.
func (b Binding) TookDefault() bool { return b.Bound == "" && b.Default != nil }

// Error is the solver's single failure kind: exactly one of the D-CAP-3 conditions was violated.
type Error struct {
	// Capability is what could not be bound.
	Capability
	// Wanted is the authored location of the entry that failed.
	Wanted string
	// Candidates are the origins that were visible and matched; nil for Unbound.
	Candidates []string
}

func (e *Error) Error() string {
	switch len(e.Candidates) {
	case 0:
		return fmt.Sprintf("%s at %s is UNBOUND: no visible provide for %s (D-CAP-1: a need must be bound)",
			e.Strength(), e.Wanted, e.Capability)
	case 1:
		return fmt.Sprintf("%s at %s is bound to %s", e.Strength(), e.Wanted, e.Candidates[0])
	default:
		return fmt.Sprintf("%s at %s matches %d visible provides of %s (%s): §8.2 allows ONE, so the load fails naming both (D-CAP-3)",
			e.Strength(), e.Wanted, len(e.Candidates), e.Capability, strings.Join(e.Candidates, ", "))
	}
}

func (e *Error) Strength() string {
	if e.Wanted == "" {
		return "capability"
	}
	return "entry"
}

// ErrAmbiguous and ErrUnbound exist so callers can test the CONDITION rather than string-match it.
var (
	ErrAmbiguous = errors.New("two visible provides of one capability (D-CAP-3)")
	ErrUnbound   = errors.New("an unbound need (D-CAP-1)")
)

// Resolve binds every Want to the one visible Provide of its capability.
//
// The rules, each a row:
//   - D-CAP-3: exactly one provide binds; two visible provides are an error naming both; an unbound
//     NEED is an error naming what was searched.
//   - D-CAP-1: a `use:` with no provide takes its `default:` and is NOT an error; a `use:` with
//     neither is also not an error — it "goes without".
//   - §8.2: a provide is fixed — nothing replaces it. So a second provide of the same capability is
//     never a preference, it is a defect.
//
// The returns are ordered by (type, name, authored) so a caller's output is stable.
func Resolve(wants []Want, provides []Provide) ([]Binding, error) {
	// Index the provides by capability. Insertion order is preserved so an ambiguous error names
	// them in the order the caller supplied, which is its stack order.
	byCap := make(map[Capability][]string, len(provides))
	for _, p := range provides {
		byCap[p.Capability] = append(byCap[p.Capability], p.Origin)
	}

	out := make([]Binding, 0, len(wants))
	var errs []error
	for _, w := range wants {
		cands := byCap[w.Capability]
		switch len(cands) {
		case 0:
			// D-CAP-1: a need must be bound; a use may go without.
			if w.Strength == Need {
				errs = append(errs, fmt.Errorf("%w: %w", ErrUnbound, &Error{Capability: w.Capability, Wanted: w.Authored}))
				continue
			}
			out = append(out, Binding{Want: w})
		case 1:
			out = append(out, Binding{Want: w, Bound: cands[0]})
		default:
			errs = append(errs, fmt.Errorf("%w: %w", ErrAmbiguous, &Error{Capability: w.Capability, Wanted: w.Authored, Candidates: cands}))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Authored < b.Authored
	})
	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}

// Union is D-CAP-6: the needs and uses of a deployment are the UNION of those of the deployment,
// its box and all their candies.
//
// A union of Wants is not a concatenation — the same entry reached twice (a candy required by both
// the box and the deployment) is one entry, and the authored location kept is the first seen, so
// the error still points somewhere real.
func Union(sets ...[]Want) []Want {
	seen := make(map[string]bool)
	var out []Want
	for _, set := range sets {
		for _, w := range set {
			key := w.String() + "|" + w.Authored
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, w)
		}
	}
	return out
}
