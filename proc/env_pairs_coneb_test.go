package proc

import (
	"strings"
	"testing"
)

// TestEnvMapToPairs covers the canonical env-map → sorted KEY=VALUE pairs helper
// (env_pairs_coneb.go). The test relocated here from proc/launch_test.go when the
// duplicate proc.SortedEnvPairs body was deleted (R3 — one implementation, spec home).
func TestEnvMapToPairs(t *testing.T) {
	got := EnvMapToPairs(map[string]string{"B": "two words", "A": "1", "C": ""})
	want := []string{"A=1", "B=two words", "C="}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("EnvMapToPairs = %#v, want %#v", got, want)
	}
	if out := EnvMapToPairs(nil); len(out) != 0 {
		t.Fatalf("EnvMapToPairs(nil) = %#v, want empty", out)
	}
}
