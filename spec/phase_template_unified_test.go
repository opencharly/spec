package spec

import "testing"

// TestFormatPhaseTemplateUnifiedInstall proves the R3 shape: the venue-agnostic
// `install` body is returned verbatim for the host venue and wrapped with the
// BuildKit cacheMounts RUN prefix for the container venue — ONE canonical body,
// venue applied at render, never two hand-maintained copies.
func TestFormatPhaseTemplateUnifiedInstall(t *testing.T) {
	f := &Format{
		Phases: &PhaseSet{
			Install: &PhaseTemplates{
				Install: "dnf install -y{{range .Packages}} {{.}}{{end}}\n",
			},
		},
	}

	host := FormatPhaseTemplate(f, PhaseInstall, VenueHostNative)
	if host != "dnf install -y{{range .Packages}} {{.}}{{end}}\n" {
		t.Errorf("host venue: want the unified body verbatim, got %q", host)
	}

	container := FormatPhaseTemplate(f, PhaseInstall, VenueContainerBuilder)
	want := "RUN {{cacheMounts .CacheMounts}} \\\n" + "dnf install -y{{range .Packages}} {{.}}{{end}}\n"
	if container != want {
		t.Errorf("container venue: want the RUN-wrapped unified body, got %q", container)
	}
}

// TestFormatPhaseTemplateVenueOverrideWins proves a venue-specific override cell
// takes precedence over the unified body — the escape hatch for a phase that
// genuinely differs by venue.
func TestFormatPhaseTemplateVenueOverrideWins(t *testing.T) {
	f := &Format{
		Phases: &PhaseSet{
			Install: &PhaseTemplates{
				Install:   "unified body\n",
				Container: "container-only override\n",
				Host:      "host-only override\n",
			},
		},
	}

	if got := FormatPhaseTemplate(f, PhaseInstall, VenueContainerBuilder); got != "container-only override\n" {
		t.Errorf("container override: want the override cell, got %q", got)
	}
	if got := FormatPhaseTemplate(f, PhaseInstall, VenueHostNative); got != "host-only override\n" {
		t.Errorf("host override: want the override cell, got %q", got)
	}
}
