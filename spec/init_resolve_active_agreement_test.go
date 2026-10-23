package spec

import "testing"

// fakeInitCandy implements only the CandyReader methods the init-resolution path
// touches (Capabilities, HasInit, RelayPorts). The embedded interface supplies the
// rest and is nil, so any other call panics loudly rather than returning a zero
// value that would quietly change what is under test.
type fakeInitCandy struct {
	CandyReader
	inits map[string]bool
	caps  *CandyCapability
}

func (f fakeInitCandy) HasInit(name string) bool       { return f.inits[name] }
func (f fakeInitCandy) RelayPorts() []int              { return nil }
func (f fakeInitCandy) Capabilities() *CandyCapability { return f.caps }

// TestResolveInitSystemAlwaysNamesAnActiveInit pins the invariant EmitInitAssembly
// depends on: a non-empty ResolveInitSystem result MUST be a key of ActiveInit.
//
// EmitInitAssembly enables a use_packaged: system unit through the resolved init
// only (`if initName == img.InitSystem`). If the resolved name is not an active
// init, that comparison matches nothing and every system unit in the image goes
// silently un-enabled — no build error, no warning, the service simply never
// starts. The invariant is what makes the narrowing safe, so it is asserted here
// rather than inferred from two functions happening to apply the same predicate.
func TestResolveInitSystemAlwaysNamesAnActiveInit(t *testing.T) {
	cases := []struct {
		name     string
		explicit string
		requires []string
		provides *CandyCapability
		wantInit string
	}{{
		// The regression: an explicit override whose capability is unmet. The
		// override branch used to return before the capability filter ran, so
		// this resolved to "systemd" while ActiveInit excluded it.
		name:     "explicit override with unmet capability",
		explicit: "systemd",
		requires: []string{"preserve_user"},
		provides: nil,
		wantInit: "",
	}, {
		name:     "explicit override with satisfied capability",
		explicit: "systemd",
		requires: []string{"preserve_user"},
		provides: &CandyCapability{PreserveUser: true},
		wantInit: "systemd",
	}, {
		name:     "explicit override, no capability required",
		explicit: "systemd",
		requires: nil,
		provides: nil,
		wantInit: "systemd",
	}, {
		name:     "auto-detect with unmet capability",
		explicit: "",
		requires: []string{"preserve_user"},
		provides: nil,
		wantInit: "",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ic := &InitConfig{Init: map[string]*ResolvedInit{
				"systemd": {RequiresCapability: tc.requires},
			}}
			layers := map[string]CandyReader{
				"c": fakeInitCandy{inits: map[string]bool{"systemd": true}, caps: tc.provides},
			}
			order := []string{"c"}

			got, def := ic.ResolveInitSystem(layers, order, tc.explicit)
			active := ic.ActiveInit(layers, order)

			if got != tc.wantInit {
				t.Errorf("ResolveInitSystem = %q, want %q", got, tc.wantInit)
			}
			// The invariant itself, independent of the expectation above.
			if got != "" {
				if _, ok := active[got]; !ok {
					t.Errorf("ResolveInitSystem returned %q, which is NOT a key of ActiveInit %v — "+
						"EmitInitAssembly would enable no system units at all", got, keysOf(active))
				}
				if def == nil {
					t.Errorf("ResolveInitSystem returned name %q with a nil def", got)
				}
			}
		})
	}
}

// TestResolveInitSystemHonorsAViableOverride is the discriminating half. The
// invariant test above pins that a resolved name is always active — it is
// satisfied by never honoring the override at all, so it stays green even if the
// override branch is deleted outright. This one fails in that case.
//
// The fixture triggers TWO inits and overrides to the one auto-detect would not
// pick: supervisord is preferred for container images, so without the override
// this composition resolves to supervisord. A single-triggered-init fixture
// cannot tell "override honored" from "auto-detect agreed".
func TestResolveInitSystemHonorsAViableOverride(t *testing.T) {
	want := &ResolvedInit{Model: "explicit-def"}
	ic := &InitConfig{Init: map[string]*ResolvedInit{
		"systemd":     want,
		"supervisord": {Model: "supervisord-def"},
	}}
	layers := map[string]CandyReader{
		"c": fakeInitCandy{inits: map[string]bool{"systemd": true, "supervisord": true}},
	}
	order := []string{"c"}

	// Control: the fixture only discriminates while auto-detect disagrees with
	// the override. If the preference rules change, this fails loudly rather than
	// letting the assertion below silently stop proving anything.
	if auto, _ := ic.ResolveInitSystem(layers, order, ""); auto != "supervisord" {
		t.Fatalf("control: ResolveInitSystem(no override) = %q, want supervisord — "+
			"fixture can no longer discriminate an honored override", auto)
	}

	name, def := ic.ResolveInitSystem(layers, order, "systemd")
	if name != "systemd" || def != want {
		t.Errorf("ResolveInitSystem(viable override) = (%q, %v), want (systemd, %v)", name, def, want)
	}
}

// TestResolveInitSystemHonorsAContainerInitHint pins the CONTAINER-venue
// selection rule: a candy's `capability: {init_system_hint: <name>}` selects that
// init over the container default, because a declared hint is more specific than
// the venue default. The fixture discriminates on the hint itself — the SAME
// composition resolves to supervisord with no hint — so the assertion fails if
// the hint branch is removed (measured: it did).
func TestResolveInitSystemHonorsAContainerInitHint(t *testing.T) {
	containerInit := &ResolvedInit{
		Model:              "file_copy",
		RequiresCapability: []string{"init_system:systemd-container"},
	}
	ic := &InitConfig{Init: map[string]*ResolvedInit{
		"supervisord":       {Model: "fragment_assembly"},
		"systemd-container": containerInit,
	}}
	layerWithHint := func(hint string) map[string]CandyReader {
		caps := &CandyCapability{InitSystemHint: hint}
		return map[string]CandyReader{
			"c": fakeInitCandy{
				inits: map[string]bool{"supervisord": true, "systemd-container": true},
				caps:  caps,
			},
		}
	}
	order := []string{"c"}

	// Control: the same composition with NO hint takes the container default.
	// Without this arm the expectation below could be satisfied by the hint
	// branch never running.
	if got, _ := ic.ResolveInitSystem(layerWithHint(""), order, ""); got != "supervisord" {
		t.Fatalf("control: ResolveInitSystem(no hint) = %q, want supervisord", got)
	}

	got, def := ic.ResolveInitSystem(layerWithHint("systemd-container"), order, "")
	if got != "systemd-container" || def != containerInit {
		t.Errorf("ResolveInitSystem(hint) = (%q, %v), want (systemd-container, %v)", got, def, containerInit)
	}

	// The box's explicit init still wins over the composition hint.
	if got, _ := ic.ResolveInitSystem(layerWithHint("systemd-container"), order, "supervisord"); got != "supervisord" {
		t.Errorf("ResolveInitSystem(hint, explicit) = %q, want supervisord (explicit override wins)", got)
	}

	// A hint that names a NON-candidate (its requirement is unmet) must fall
	// through to the venue default, never resolve to a name ActiveInit excludes.
	unmet := &InitConfig{Init: map[string]*ResolvedInit{
		"supervisord":       {Model: "fragment_assembly"},
		"systemd-container": {Model: "file_copy", RequiresCapability: []string{"preserve_user"}},
	}}
	if got, _ := unmet.ResolveInitSystem(layerWithHint("systemd-container"), order, ""); got != "supervisord" {
		t.Errorf("ResolveInitSystem(hint with unmet requirement) = %q, want supervisord", got)
	}
}

func keysOf(m map[string]*ResolvedInit) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
