package needsolver

import (
	"errors"
	"strings"
	"testing"
)

// D-CAP-3: "One solver binds every need and use". A need with exactly one visible provide binds to
// it, and the binding names the ORIGIN so a reader can tell which stack member supplied it.
func TestOneVisibleProvideBinds(t *testing.T) {
	got, err := Resolve(
		[]Want{{Capability: Capability{"secret", "db_password"}, Strength: Need, Authored: "candy:app"}},
		[]Provide{{Capability: Capability{"secret", "db_password"}, Origin: "candy:db"}},
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got) != 1 || got[0].Bound != "candy:db" || got[0].TookDefault() {
		t.Fatalf("bound = %+v, want candy:db", got)
	}
}

// D-CAP-1: "a `need:` must be bound and fails the load when it is not". The error must NAME WHAT
// WAS SEARCHED, so the author knows which capability is missing rather than that something is.
func TestAnUnboundNeedFailsAndNamesWhatWasSearched(t *testing.T) {
	_, err := Resolve(
		[]Want{{Capability: Capability{"secret", "db_password"}, Strength: Need, Authored: "candy:app"}},
		nil,
	)
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("err = %v, want ErrUnbound", err)
	}
	if !strings.Contains(err.Error(), "secret:db_password") || !strings.Contains(err.Error(), "candy:app") {
		t.Fatalf("error does not name the capability and the author site: %v", err)
	}
}

// D-CAP-1, the other half: "a `use:` is bound when a provide exists and takes its `default:`, or goes
// without, otherwise." An unbound USE is NOT an error — that is the whole difference from a need.
func TestAnUnboundUseTakesItsDefaultOrGoesWithout(t *testing.T) {
	def := "30"
	got, err := Resolve(
		[]Want{
			{Capability: Capability{"endpoint", "metrics"}, Strength: Use, Default: &def, Authored: "candy:a"},
			{Capability: Capability{"endpoint", "traces"}, Strength: Use, Authored: "candy:b"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("an unbound use must not fail: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d bindings, want 2", len(got))
	}
	withDefault := 0
	for _, b := range got {
		if b.TookDefault() {
			withDefault++
			if b.Default == nil || *b.Default != "30" {
				t.Fatalf("default not carried: %+v", b)
			}
		}
	}
	if withDefault != 1 {
		t.Fatalf("%d bindings took a default, want exactly 1 (the one that HAS one)", withDefault)
	}
}

// D-CAP-3: "Two visible provides of one capability are an error naming both." §8.2 adds why: "A
// provide is fixed: nothing replaces it" — so a second provide is a defect, never a preference.
func TestTwoVisibleProvidesFailNamingBoth(t *testing.T) {
	_, err := Resolve(
		[]Want{{Capability: Capability{"secret", "db_password"}, Strength: Need, Authored: "candy:app"}},
		[]Provide{
			{Capability: Capability{"secret", "db_password"}, Origin: "candy:db-a"},
			{Capability: Capability{"secret", "db_password"}, Origin: "candy:db-b"},
		},
	)
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("err = %v, want ErrAmbiguous", err)
	}
	for _, want := range []string{"candy:db-a", "candy:db-b"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error must name BOTH providers; %q missing from %v", want, err)
		}
	}
}

// D-CAP-6: the needs and uses of a deployment are the UNION of those of the deployment, its box and
// all their candies. Reaching the same authored entry twice is ONE entry — otherwise the same
// missing secret would be reported once per path that led to it.
func TestUnionIsASetNotAConcatenation(t *testing.T) {
	box := []Want{{Capability: Capability{"secret", "a"}, Strength: Need, Authored: "candy:shared"}}
	deploy := []Want{
		{Capability: Capability{"secret", "a"}, Strength: Need, Authored: "candy:shared"},
		{Capability: Capability{"secret", "b"}, Strength: Need, Authored: "deploy:main"},
	}
	got := Union(box, deploy)
	if len(got) != 2 {
		t.Fatalf("union = %d entries, want 2 (the shared candy counted once): %+v", len(got), got)
	}
}

// §8.2 rule 1: a provide is only visible to what can see it. The solver resolves the SET IT IS
// HANDED — so a need whose provide lives outside that set is unbound, not silently satisfied by a
// provider the caller never made visible. This is the property that keeps the solver honest about
// the tree the caller actually knows.
func TestTheSolverOnlySeesWhatItIsHanded(t *testing.T) {
	want := []Want{{Capability: Capability{"secret", "x"}, Strength: Need, Authored: "candy:inner"}}
	if _, err := Resolve(want, nil); !errors.Is(err, ErrUnbound) {
		t.Fatalf("a provide the caller did not hand in must not bind: %v", err)
	}
	// The same want binds once the caller makes that provide visible — which is §8.2's whole point.
	if _, err := Resolve(want, []Provide{{Capability: Capability{"secret", "x"}, Origin: "deploy:outer"}}); err != nil {
		t.Fatalf("a visible provide must bind: %v", err)
	}
}
