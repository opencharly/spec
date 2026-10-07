package spec

import (
	"slices"
	"testing"
)

// TestLocalPkgInstallStepIR exercises the IR contract for LocalPkgInstallStep
// (relocated from charly/localpkg_test.go, K3 cone2 test closure): kind, scope
// (system), venue (host-native), gate (none), reverse (no ledger ops — like
// apk) — a pure spec.LocalPkgInstallStep test with zero charly-core
// dependency, and no prior duplicate found anywhere in the tree.
func TestLocalPkgInstallStepIR(t *testing.T) {
	s := &LocalPkgInstallStep{PackageName: "charly", CandyName: "charly"}
	if s.Kind() != StepKindLocalPkgInstall {
		t.Errorf("Kind() = %q, want %q", s.Kind(), StepKindLocalPkgInstall)
	}
	if s.Scope() != ScopeSystem {
		t.Errorf("Scope() = %v, want ScopeSystem", s.Scope())
	}
	if s.Venue() != VenueHostNative {
		t.Errorf("Venue() = %v, want VenueHostNative", s.Venue())
	}
	if s.RequiresGate() != GateNone {
		t.Errorf("RequiresGate() = %v, want GateNone", s.RequiresGate())
	}
	if s.Reverse() != nil {
		t.Errorf("Reverse() = %v, want nil (OS package is the substrate's own, not ledger-reversed)", s.Reverse())
	}
}

// TestSystemPackagesStepReverseInstalledDelta pins the teardown delta contract:
// Reverse() prefers the execution-time Installed set over the declared Packages,
// with a nil-vs-empty distinction. nil falls back to Packages (prior behaviour);
// a NON-nil EMPTY slice is authoritative — every declared package was already
// present, so NO package-remove op is recorded.
func TestSystemPackagesStepReverseInstalledDelta(t *testing.T) {
	t.Run("nil Installed falls back to Packages", func(t *testing.T) {
		s := &SystemPackagesStep{
			Format:   "pac",
			Phase:    PhaseInstall,
			Packages: []string{"kind", "curl"},
		}
		ops := s.Reverse()
		if len(ops) != 1 || ops[0].Kind != ReverseOpPackageRemove {
			t.Fatalf("Reverse() = %+v, want exactly one package-remove op", ops)
		}
		if want := []string{"kind", "curl"}; !slices.Equal(ops[0].Targets, want) {
			t.Errorf("Targets = %v, want the declared Packages %v", ops[0].Targets, want)
		}
	})

	t.Run("non-empty Installed wins over Packages", func(t *testing.T) {
		s := &SystemPackagesStep{
			Format:    "pac",
			Phase:     PhaseInstall,
			Packages:  []string{"kind", "curl", "iptables"},
			Installed: []string{"kind"},
		}
		ops := s.Reverse()
		if len(ops) != 1 || ops[0].Kind != ReverseOpPackageRemove {
			t.Fatalf("Reverse() = %+v, want exactly one package-remove op", ops)
		}
		if want := []string{"kind"}; !slices.Equal(ops[0].Targets, want) {
			t.Errorf("Targets = %v, want the Installed delta %v", ops[0].Targets, want)
		}
	})

	t.Run("empty non-nil Installed records nothing", func(t *testing.T) {
		s := &SystemPackagesStep{
			Format:    "pac",
			Phase:     PhaseInstall,
			Packages:  []string{"curl", "iptables"},
			Installed: []string{},
		}
		if ops := s.Reverse(); len(ops) != 0 {
			t.Errorf("Reverse() = %+v, want no ops (every declared package already present)", ops)
		}
	})

	t.Run("non-install phase is unaffected by Installed", func(t *testing.T) {
		s := &SystemPackagesStep{
			Format:    "pac",
			Phase:     PhasePrepare,
			Packages:  []string{"curl"},
			Installed: []string{"curl"},
			Copr:      []string{"coolercontrol/coolercontrol"},
		}
		ops := s.Reverse()
		if len(ops) != 1 || ops[0].Kind != ReverseOpCoprDisable {
			t.Fatalf("Reverse() = %+v, want exactly one copr-disable op (the Prepare arm is untouched)", ops)
		}
		if want := []string{"coolercontrol/coolercontrol"}; !slices.Equal(ops[0].Targets, want) {
			t.Errorf("Targets = %v, want %v", ops[0].Targets, want)
		}
	})
}
