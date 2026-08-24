package spec

// phase_template.go — the (phase, venue) → template-string resolvers for the build-vocabulary
// Format / Builder. Pure over the CUE-sourced spec types (Format, Builder, Phase/Venue enums).
// #55 import-purity: relocated from sdk/buildkit DOWN to spec (the value leaf) so charly core reads
// them over its spec+proto-only import surface; sdk/buildkit keeps thin var-forwarders for its
// build-render callers.

// FormatPhaseTemplate looks up the template string for a (phase, venue)
// lookup. The phase: block is the single source of truth — the legacy
// top-level InstallTemplate field was removed (R5) and its content migrated
// into phase.install, so there is no fallback arm. A lookup returns "" when
// the requested cell is absent.
//
// R3 (one canonical body, venue applied at render): the install cell is the
// venue-agnostic body written with `&& \` continuations (valid plain shell AND
// inside a Dockerfile RUN). The container venue wraps it with the BuildKit
// cacheMounts RUN prefix; the host venue returns it verbatim. Venue-specific
// OVERRIDES (host/container) take precedence when present, so a phase that
// genuinely differs by venue keeps its two cells without forcing a second
// canonical copy.
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
				if pt.Install != "" {
					return pt.Install
				}
			case VenueContainerBuilder:
				if pt.Container != "" {
					return pt.Container
				}
				if pt.Install != "" {
					return "RUN {{cacheMounts .CacheMounts}} \\\n" + pt.Install
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
