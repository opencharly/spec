package spec

import "testing"

// TestBuilderPackageSurvivesTheStepView pins that a BuilderStep's plugin-declared package output
// crosses the wire: StepToView → StepFromView restores it, so a venue that receives the plan as a
// view still installs the packages through local_pkg and still reports system scope
// (opencharly/sdk#608). A dropped field would silently turn an aur step into a user-scope payload.
func TestBuilderPackageSurvivesTheStepView(t *testing.T) {
	pkg := &BuilderPackage{Format: "pac", Dir: "/tmp/aur-pkgs/kde-shell", Glob: "*.pkg.tar.zst"}
	step, err := StepFromView(StepToView(&BuilderStep{Builder: "aur", CandyName: "kde-shell", Package: pkg}))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := step.(*BuilderStep)
	if !ok || got.Package == nil || *got.Package != *pkg {
		t.Fatalf("round-tripped BuilderStep = %+v, want Package %+v", step, pkg)
	}
	if got.Scope() != ScopeSystem {
		t.Errorf("round-tripped aur step scope = %v, want system", got.Scope())
	}
}
