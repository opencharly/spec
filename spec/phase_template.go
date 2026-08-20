package spec

// phase_template.go — the (phase, venue) → template-string resolvers for the build-vocabulary
// Format / Builder. Pure over the CUE-sourced spec types (Format, Builder, Phase/Venue enums).
// #55 import-purity: relocated from sdk/buildkit DOWN to spec (the value leaf) so charly core reads
// them over its spec+proto-only import surface; sdk/buildkit keeps thin var-forwarders for its
// build-render callers.

// FormatPhaseTemplate looks up the template string for a (phase, venue)
// lookup. The phase: block is the single source of truth — the legacy
// top-level InstallTemplate field was removed (R5) and its content migrated
// into phase.install.container, so there is no fallback arm. A lookup
// returns "" when the requested cell is absent.
func FormatPhaseTemplate(f *Format, phase Phase, venue Venue) string {
	if f == nil {
		return ""
	}
	if f.Phases != nil {
		var pt *PhaseTemplates
		switch phase {
		case PhasePrepare:
			pt = f.Phases.Prepare
		case PhaseInstall:
			pt = f.Phases.Install
		case PhaseCleanup:
			pt = f.Phases.Cleanup
		}
		if pt != nil {
			switch venue {
			case VenueHostNative:
				if pt.Host != "" {
					return pt.Host
				}
			case VenueContainerBuilder:
				if pt.Container != "" {
					return pt.Container
				}
			}
		}
	}
	return ""
}

// BuilderPhaseTemplate is the Builder analog of FormatPhaseTemplate. The
// phase: block is the single source of truth — the legacy inline
// InstallTemplate field was removed (R5) and its content migrated into
// phase.install.container, so there is no fallback arm. Multi-stage
// builders render via their plugin's OpResolve (kit.BuilderResolve), NOT
// this resolver.
func BuilderPhaseTemplate(b *Builder, phase Phase, venue Venue) string {
	if b == nil {
		return ""
	}
	if b.Phases != nil {
		var pt *PhaseTemplates
		switch phase {
		case PhasePrepare:
			pt = b.Phases.Prepare
		case PhaseInstall:
			pt = b.Phases.Install
		case PhaseCleanup:
			pt = b.Phases.Cleanup
		}
		if pt != nil {
			switch venue {
			case VenueHostNative:
				if pt.Host != "" {
					return pt.Host
				}
			case VenueContainerBuilder:
				if pt.Container != "" {
					return pt.Container
				}
			}
		}
	}
	return ""
}
