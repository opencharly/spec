package checkstep

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// stepStub is a minimal StepProvider. It exists to lock the interface's EXACT method set
// at compile time: StepKind returns the internal IR spec.StepKind (NOT a candy-local
// string enum), and MaterializeStep takes the op + the four host-resolved ctx scalars and
// returns a real spec.InstallStep. A later edit that re-narrows or re-widens the contract
// fails this file to compile — the coverage that fails without the C7 cutover.
type stepStub struct{ kind spec.StepKind }

func (s stepStub) StepKind() spec.StepKind { return s.kind }

func (s stepStub) MaterializeStep(_ *spec.Op, _, _, pkgFormat string, distroTags []string) spec.InstallStep {
	return &spec.SystemPackagesStep{
		Format:   pkgFormat,
		Phase:    spec.PhaseInstall,
		Packages: []string{ResolvePackageName("openssh", map[string]string{"fedora": "openssh-server"}, distroTags)},
	}
}

var _ StepProvider = stepStub{}

func TestStepProviderContract(t *testing.T) {
	sp := stepStub{kind: spec.StepKindSystemPackages}
	if got := sp.StepKind(); got != spec.StepKindSystemPackages {
		t.Fatalf("StepKind = %v, want %v", got, spec.StepKindSystemPackages)
	}
	step := sp.MaterializeStep(&spec.Op{}, "1000", "net", "rpm", []string{"fedora:43", "fedora"})
	if step.Kind() != spec.StepKindSystemPackages {
		t.Fatalf("materialized kind = %v, want %v", step.Kind(), spec.StepKindSystemPackages)
	}
	sps, ok := step.(*spec.SystemPackagesStep)
	if !ok || sps.Format != "rpm" || sps.Phase != spec.PhaseInstall || len(sps.Packages) != 1 || sps.Packages[0] != "openssh-server" {
		t.Fatalf("materialized step = %+v, want SystemPackagesStep{Format:rpm Phase:Install Packages:[openssh-server]}", step)
	}
}

func TestResolvePackageName(t *testing.T) {
	if got := ResolvePackageName("openssh", map[string]string{"fedora": "openssh-server"}, []string{"fedora:43", "fedora"}); got != "openssh-server" {
		t.Fatalf("resolve = %q, want openssh-server", got)
	}
	if got := ResolvePackageName("bash", nil, []string{"fedora"}); got != "bash" {
		t.Fatalf("resolve bare = %q, want bash", got)
	}
}
