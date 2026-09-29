package spec

// resolve_opts.go — the loader-config OPTIONS (ResolveOpts), the scan/load options threaded through
// the candy scan + project resolution. Relocated here from sdk/loaderkit (#55 loader cascade) so the
// ~14 charly-core call sites that only NAME this options struct reach it through the dedicated spec
// module and drop their loaderkit import. Its fields are the project build-vocabulary configs
// (*InitConfig / *DistroConfig / *BuilderConfig) — all native spec types since #72, carrying their
// own resolve methods (init_config_methods.go / distro_config_methods.go), so ResolveOpts references
// only sibling spec types and pulls in no mechanism package. DISTINCT from buildkit.ResolveOpts (the
// build-resolve options): this is the SCAN/LOAD options the candy scan + project validation consume;
// the buildkit resolvers never read ExtraCandyRefs/InitCfg/RequestedBoxes.

// ExtraCandyRef (schema/buildwire.cue's `#ExtraCandyRef`, generated into
// cue_types_gen.go) is a candy ref collected IN ADDITION to the closure, together with the
// composition SCOPE that named it. The scope travels WITH the ref so the version arbiter
// (loaderkit.PickCandyVersion's scopeConflicts) never has to GUESS it. It is the same
// "every producer states where its data belongs" discipline the scan's Warn sink follows.
//
// WHY THIS SHAPE EXISTS: the ref's ORIGIN is lost the moment a bare string is appended to a
// flat []string, and the collector must then invent a label. That invention is exactly how
// opencharly/charly#739 was produced — every deploy's add_candy refs were tagged with the
// CONSTANT "deploy=add_candy", so every deploy in the project collapsed into ONE bogus scope
// and ~724 false version conflicts were reported. Making the scope part of the data removes
// the guess: a deploy's add_candy refs carry that deploy's BOX scope, and a local candy's raw
// require:/candy: deps carry that LAYER's scope.
//
// (The type itself is CUE-sourced — SDD, one schema owns the wire shape. Ref is the verbatim
// candy ref (a bare name or a qualified "@github…" ref); Scope is the composition scope that
// named it ("box=<qualified-name>", "layer=<candy>", "kind:local=<template>"). Two refs
// sharing a scope are one composition and may legitimately be arbitrated against each other;
// two refs in different scopes are independent compositions. An EMPTY scope means "no scope"
// — a context with no owning composition — and never conflicts.)

// ExtraCandyRefStrings projects the typed list back to its raw ref strings for the consumers
// that need ONLY the ref word and not its composition scope — today that is the plugin-word
// collector (`charly`'s collectReferencedPluginWords, reached via host_build_buildengine.go)
// and `plugin-build`'s resolve_project_word.go. It is a PROJECTION over the typed list, never a
// second source of truth: the scope is not lost because the CALLER re-attaches it when it knows
// its box. (The wire fields all carry []ExtraCandyRef — this is not a legacy transport path.)
func ExtraCandyRefStrings(refs []ExtraCandyRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Ref)
	}
	return out
}

// ScopedExtraCandyRefs tags each raw ref with the same composition scope. It is the ONE
// constructor a caller uses to state where its extra refs belong — a deploy's add_candy refs
// take the deploy's box scope, never a constant.
func ScopedExtraCandyRefs(scope string, refs ...string) []ExtraCandyRef {
	out := make([]ExtraCandyRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, ExtraCandyRef{Ref: r, Scope: scope})
	}
	return out
}

// The composition-SCOPE label grammar. ONE owner (R3): the scope strings an ExtraCandyRef
// carries and the reachability walk's referrers use are built here, so a producer and a
// consumer can never disagree on the wire form.

// BoxScope is the scope of a box's own candy closure (and of a deploy's `add_candy:` refs,
// which are part of the deploy's box composition).
func BoxScope(box string) string { return "box=" + box }

// LayerScope is the scope of a layer's OWN authored require:/candy: deps — used both for a
// shared layer's attribution and for a local candy's harvested raw refs.
func LayerScope(candy string) string { return "layer=" + candy }

// KindLocalScope is the scope of a `kind: local` template's candy list.
func KindLocalScope(template string) string { return "kind:local=" + template }

// ResolveOpts carries the scan/load options threaded through the candy scan + project resolution.
type ResolveOpts struct {
	IncludeDisabled      bool            // skip the `enabled: false` check
	IncludeDisabledNames map[string]bool // when non-empty, scope IncludeDisabled to these names only
	// RequestedBoxes are the explicit build targets (`charly box build <name>`). A qualified name
	// here (e.g. `charly.arch-builder`) is pulled into the resolved set even when it isn't reachable
	// as a base/builder of a root image — so a namespaced image can be an on-demand build target, not
	// only a transitive base. Bare names are ignored here (they resolve through the root loop).
	RequestedBoxes []string
	// ExtraCandyRefs are candy refs to collect IN ADDITION to the image/builder/kind:local-template
	// closure, each carrying the composition SCOPE that named it. Two callers append here:
	//   - a DEPLOY's `add_candy:` candies (scope "box=<the deploy's box>");
	//   - `WithLocalRawRefs`' per-local-candy require:/candy: deps (scope "layer=<candy>").
	// The ref's ORIGIN is data, not a label the collector invents — a flat []string lost it and
	// collapsed every composition into one scope (opencharly/charly#739).
	// NEVER read by the buildkit resolvers — consumed solely by the candy scan.
	ExtraCandyRefs []ExtraCandyRef
	// InitCfg is the project init: vocabulary (W9), threaded through so the candy scan can run the
	// cross-candy init-system host-completion pass (PopulateCandyInitSystem) BEFORE wrapping each
	// candy into the FINAL CandyReader. A caller that leaves this nil skips the pass (correct only for
	// a caller with no init-aware consumer downstream). NEVER read by the buildkit resolvers.
	InitCfg *InitConfig
	// DistroCfg / BuilderCfg are the project's build vocabulary (distro:/builder:), threaded through
	// so a resolve does not re-run the project load on every call (a caller with the triple, or a
	// multi-box loop, sets it once and skips the redundant reload; nil is byte-identical fallback).
	DistroCfg  *DistroConfig
	BuilderCfg *BuilderConfig
}

// BoxResolveOpts builds the ResolveOpts that scope a generate/build to a set of explicitly-named
// boxes. It is the SINGLE source of the box-selection rule (R3) for `charly box build` and
// `charly box generate` alike: an empty slice means "all enabled boxes" (no scoping); a non-empty
// slice pins those names into the resolved set (RequestedBoxes) and, when includeDisabled is set,
// relaxes the `enabled: false` gate for exactly those names (IncludeDisabledNames) so the override
// never widens the working set globally. Callers pass boxes already run through
// buildkit.NormalizeBoxArgs.
//
// It lived as charly's private `boxResolveOpts` until K-wave 2 cone R1 (A2). candy/plugin-build now
// builds the same value plugin-side to drive CollectRemoteRefsOpts itself, so the rule moves to the
// shared fabric module both sides import rather than being duplicated across the boundary — a pure
// ResolveOpts constructor over sibling spec types, pulling in no mechanism package.
func BoxResolveOpts(boxes []string, includeDisabled bool) ResolveOpts {
	opts := ResolveOpts{IncludeDisabled: includeDisabled}
	if len(boxes) == 0 {
		return opts
	}
	opts.RequestedBoxes = boxes
	if includeDisabled {
		opts.IncludeDisabledNames = make(map[string]bool, len(boxes))
		for _, name := range boxes {
			opts.IncludeDisabledNames[name] = true
		}
	}
	return opts
}

// WithLocalRawRefs returns opts with every local candy's RAW (pre-finalize) require:/candy: refs
// appended to ExtraCandyRefs, each tagged with its ORIGIN scope ("layer=<candy>"). CollectRemoteRefsOpts's own "candy manifest require:/candy:" walk
// reads CandyView.Require/.IncludedCandy — the FINALIZED bare-string wire form (FinalizeCandyRefs
// strips a "@repo:vTAG" pin down to the bare graph-topology name; correct for its OWN consumers,
// ExpandCandy/ResolveCandyOrder, which are version-agnostic). Feeding that walk a wrapped view
// therefore leaves it structurally UNABLE to discover a local candy's pinned remote dep at all (a
// bare name never looks remote to IsRemoteCandyRef) — the confirmed root cause of a "depends:
// unknown candy" crash a live box/cachyos generate surfaced (a local candy's require: pins a
// remote plugin candy). So the raw pre-finalize refs (still carrying the full pin, from
// ScannedCandy.Refs) are harvested here and fed in as ExtraCandyRefs — the SAME mechanism a
// deploy's add_candy: already uses to reach a ref no base/builder/require edge would otherwise
// surface. A local (non-remote) ref is a harmless no-op (IsRemoteCandyRef gates it).
//
// SCOPE: each harvested ref is tagged with the LAYER that owns the dependency ("layer=<candy>") —
// the composition that authored the require:/candy: edge. That is the true origin; a constant
// would erase it (opencharly/charly#739).
//
// Relocated from charly/layers.go in K-wave 2 cone R1 (A2) for the same reason as BoxResolveOpts:
// candy/plugin-build's own CollectRemoteRefsOpts call needs the identical augmentation, and a
// second copy across the module boundary is the R3 duplicate this program removes.
func WithLocalRawRefs(opts ResolveOpts, localScanned map[string]ScannedCandy) ResolveOpts {
	extraRefs := append([]ExtraCandyRef(nil), opts.ExtraCandyRefs...)
	for candyName, sc := range localScanned {
		scope := LayerScope(candyName)
		for _, dep := range sc.Refs.Require {
			extraRefs = append(extraRefs, ExtraCandyRef{Ref: dep.Raw, Scope: scope})
		}
		for _, dep := range sc.Refs.IncludedCandy {
			extraRefs = append(extraRefs, ExtraCandyRef{Ref: dep.Raw, Scope: scope})
		}
	}
	opts.ExtraCandyRefs = extraRefs
	return opts
}

// ShouldIncludeDisabled reports whether name's disabled gate should be bypassed under opts.
// Centralizes the IncludeDisabled + IncludeDisabledNames interaction so call sites stay simple.
func (opts ResolveOpts) ShouldIncludeDisabled(name string) bool {
	if !opts.IncludeDisabled {
		return false
	}
	if len(opts.IncludeDisabledNames) == 0 {
		return true
	}
	return opts.IncludeDisabledNames[name]
}
