package schema_test

// hand_state_lockstep_test.go — the control for spec/hand_state_types.go, whose header claims
// "every field here has a matching CUE field".
//
// That file exists because `cue exp gengotypes` cannot express two shapes faithfully: `PortSpec`
// (an int|"proto:port" disjunction that degrades to `any`) and `#VmDeployState` (an open `...`
// record that degrades to `map[string]any`, so the CUE def is `@go(-)`'d and the concrete Go
// struct is mirrored by hand). A hand mirror is a promise; this test is the promise's control —
// it fails the moment either side gains a field the other does not have.

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/spec"
)

// cueFields returns the optional/required field names declared directly in the named CUE def.
func cueFields(t *testing.T, def string) []string {
	t.Helper()
	src, err := schema.FS.ReadFile("deploy.cue")
	if err != nil {
		t.Fatalf("reading deploy.cue: %v", err)
	}
	lines := strings.Split(string(src), "\n")
	start := -1
	for i, l := range lines {
		if l == def+": {" || strings.HasPrefix(l, def+": {") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s not found in deploy.cue", def)
	}
	field := regexp.MustCompile(`^\t([a-z_][a-z0-9_]*)\??\s*:`)
	var out []string
	for _, l := range lines[start+1:] {
		if strings.HasPrefix(l, "}") {
			break
		}
		if m := field.FindStringSubmatch(l); m != nil {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}

// goFields returns the yaml tag names of a struct type, which are the authored field names.
func goFields(t *testing.T, v any) []string {
	t.Helper()
	typ := reflect.TypeOf(v)
	var out []string
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		name := strings.Split(tag, ",")[0]
		if name == "" {
			t.Fatalf("%s.%s has no yaml tag", typ.Name(), typ.Field(i).Name)
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func assertSameFields(t *testing.T, what string, cue, goSide []string) {
	t.Helper()
	if reflect.DeepEqual(cue, goSide) {
		return
	}
	var onlyCUE, onlyGo []string
	g := map[string]bool{}
	for _, f := range goSide {
		g[f] = true
	}
	c := map[string]bool{}
	for _, f := range cue {
		c[f] = true
	}
	for _, f := range cue {
		if !g[f] {
			onlyCUE = append(onlyCUE, f)
		}
	}
	for _, f := range goSide {
		if !c[f] {
			onlyGo = append(onlyGo, f)
		}
	}
	t.Errorf("%s is out of lockstep: declared in CUE only %v; declared in Go only %v", what, onlyCUE, onlyGo)
}

func TestVmDeployStateMirrorsItsCueDef(t *testing.T) {
	assertSameFields(t, "spec.VmDeployState vs #VmDeployState",
		cueFields(t, "#VmDeployState"), goFields(t, spec.VmDeployState{}))
	assertSameFields(t, "spec.EphemeralRuntime vs #VmDeployState.ephemeral",
		cueFields(t, "#EphemeralRuntime"), goFields(t, spec.EphemeralRuntime{}))
	assertSameFields(t, "spec.VmKeyInjectionResolved vs #VmDeployState.key_injection_resolved",
		cueFields(t, "#VmKeyInjectionResolved"), goFields(t, spec.VmKeyInjectionResolved{}))
	assertSameFields(t, "spec.VmSnapshotState vs #VmDeployState.snapshot",
		cueFields(t, "#VmSnapshotState"), goFields(t, spec.VmSnapshotState{}))
}
