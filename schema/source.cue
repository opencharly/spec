package schema

// source.cue — the `source` KIND's definition. #Source validates ONE value of the
// `source:` map: an external starting point, one origin plus the `os` it provides.
//
// WHY THIS FILE EXISTS (SDD, D-LAW-4 / D-SCH-1): `source:` has been a DECLARED kind since
// DESIGN.cue:283 with its shape written at DESIGN.cue:799-809 — and nothing implemented
// it. Every box starting point therefore arrived in one of FOUR ad-hoc spellings
// (`base:`, `from: builder:<word>` + `bootstrap_builder_image`, a VM template `source:`
// block, and the retired clone/imported arms). This file is the one node those
// spellings become.
//
// The shape is MIRRORED VERBATIM from DESIGN.cue:799-809 and :436. Nothing here is
// designed.
//
// D-SRC-1 (exactly one origin, remote origins pinned by sha256) is enforced by
// #SourceAlt being a key-tagged S4 union: `or([...])` admits exactly one arm, and `diskc`
// / `iso` carry a #Download whose sha256 is REQUIRED, so an unpinned remote is
// unrepresentable rather than merely discouraged.
//
// D-SRC-2: `os` is REQUIRED on every origin. That is why this file and os.cue are one
// change — an origin that does not name its os cannot say what converts it into the
// other form (§16.4), so the reference would be dangling without the kind existing.

// #ImageRef — an OCI image reference: registry/repo, an optional tag, an optional
// digest. Distinct from #OciRef (which carries an explicit `oci://` scheme) because a
// source's `oci:` origin names an image the way `podman pull` does, with no scheme.
#ImageRef: =~"^[a-z0-9.-]+(:[0-9]+)?/[a-z0-9._/-]+(:[A-Za-z0-9._-]+)?(@sha256:[0-9a-f]{64})?$"

// The fields EVERY origin carries, whichever it is. Spelled out in each arm rather than
// spread from a helper: this module's schema does not use definition-spread (`#X...`)
// anywhere, because CUE requires `@experiment(explicitopen)` for it and the module
// carries no experiment flags. Keeping the four arms explicit is also what makes the
// "exactly one origin" requirement readable at the point where it is enforced.

// #SourceIso — a disk image plus the installer's own answer file.
#SourceIso: {url!: #Url, sha256!: #Sha256, config?: #Opaque} // S10: the installer's own answer file

// #SourceBootstrap — a root filesystem made from scratch by a builder.
#SourceBootstrap: {builder!: #Ref @ref(builder), package?: #PackageMap}

// #PackageMap — the packages a bootstrap asks for, keyed by the entry that wants them.
#PackageMap: {[string]: [...#Package]}

// #SourceAlt — the four origins, as an S4 key-tagged union. Exactly one is admitted,
// which is D-SRC-1's "exactly one origin" made unrepresentable-otherwise.
#SourceAlt: {
	oci: {
		oci!:         #ImageRef
		description!: #Text
		os!:          #Ref @ref(os)
	}
	disk: {
		disk!:        #Download
		description!: #Text
		os!:          #Ref @ref(os)
	}
	iso: {
		iso!:         #SourceIso
		description!: #Text
		os!:          #Ref @ref(os)
	}
	bootstrap: {
		bootstrap!:   #SourceBootstrap
		description!: #Text
		os!:          #Ref @ref(os)
	}
}

// #Source — one of them.
#Source: or([for _, a in #SourceAlt {a}])
