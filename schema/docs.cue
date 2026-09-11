// CUE schema for the `docs` KIND — the ONE docs-site generation CONFIG entity per repo
// (everything `charly docs generate` needs, declared in the repo's charly.yml).
// A `docs:` node is a top-level entity carrying the `docs` kind discriminator —
// `<name>: {docs: <#DocsConfig body>}` — discovered by the SAME recursive walk
// (sdk/candywalk, readKinds-style projection) that surfaces `skill:` / `hook:` /
// `marketplace:` entities. Classification mirrors those three EXACTLY (R3 — NO
// special-casing for docs anywhere): `docs` is NOT a reserved document directive
// (#NodeDoc keeps it on the generic `#Node` spine — a non-directive top-level key
// whose value is a node-shaped mapping is accepted structurally by the unified
// loader gate), and the `docs` kind word is recognized dynamically by its
// registered ClassKind provider host-side, exactly like every other plugin-provided
// kind. This file owns only the closed VALUE contract + the generated spec types.
//
// Semantics (consumed by candy/plugin-docs):
//   - sources.compiled — the plugin corpus COMPILED INTO the charly binary: where
//     the compiled-in plugin manifest (providers: + compiled_plugins:) lives and
//     which go.mod pins it (release_repos resolve via ITS require: else the repo's
//     latest CalVer tag at generation time).
//   - marketplace / projections / output / landing / gates — the marketplace layout,
//     the reference-projection toggles, the hand-authored tree the generated
//     reference pages splice into, the landing-page source, and the post-generation
//     site/sidebar link gates + prune.
#DocsConfig: close({
	sources?:     #DocsSources
	marketplace?: #DocsMarketplace
	projections?: #DocsProjections
	output?:      #DocsOutput
	landing?:     #DocsLanding
	gates?:       #DocsGates
})

// #DocsSources — where the reference documentation is GENERATED from.
#DocsSources: close({
	// compiled — the plugins compiled into the charly binary (the reference corpus:
	// the plugin/kind/verb surfaces they provide are the docs' source material).
	compiled?: #DocsCompiled
	// release_repos — bare candy repo names (e.g. plugin-review, plugin-pipeline)
	// resolved like a go.mod require: entry — from the compiled corpus' go.mod when
	// present, else the repo's latest CalVer tag at generation time.
	release_repos?: [...(string & !="")] @go(ReleaseRepos,type=[]string)
	// extra_repos — additional bare repo names fetched + documented OUTSIDE the
	// compiled corpus (e.g. plugin-gh), same go.mod-require-else-tag resolution.
	extra_repos?: [...(string & !="")] @go(ExtraRepos,type=[]string)
})

// #DocsCompiled — the compiled-in plugin corpus pointer.
#DocsCompiled: close({
	enabled?: *true | bool
	// compiled_plugins_path — the charly.yml whose compiled_plugins: list + providers:
	// manifest declare the binary's in-proc corpus (default: the charly repo's own
	// charly.yml at the umbrella root).
	compiled_plugins_path?: *"charly/charly.yml" | string & !="" @go(CompiledPluginsPath)
	// go_mod_path — the go.mod require: source for the release_repos resolution.
	go_mod_path?: *"charly/go.mod" | string & !="" @go(GoModPath)
})

// #DocsMarketplace — the marketplace repo layout (the `marketplace generate` input).
#DocsMarketplace: close({
	// path — the marketplace repo's root directory (its candy/ + box/ walks source
	// the skill:/hook:/marketplace: corpus the reference pages link to).
	path?: *"marketplace" | string & !=""
})

// #DocsProjections — the reference projections the generator emits (all on by
// default; a projection's page set is generated only when its toggle is set).
#DocsProjections: close({
	recipes?:   *true | bool
	cli?:       *true | bool
	providers?: *true | bool
	candy?:     *true | bool
	box?:       *true | bool
	plugin?:    *true | bool
	landing?:   *true | bool
})

// #DocsOutput — the hand-authored doc tree the generated pages are spliced into.
#DocsOutput: close({
	// hand_authored — the hand-written docs roots (start/concepts/guides/…); the
	// generator merges the generated reference pages beneath them and leaves the
	// trees it does not own untouched.
	hand_authored?: [...(string & !="")] @go(HandAuthored,type=[]string)
})

// #DocsLanding — the landing page source.
#DocsLanding: close({
	// readme — the repo-root README that anchors the landing page.
	readme?: *"README.md" | string & !=""
})

// #DocsGates — the post-generation shape gates (each runs on every generation and
// FAILS the generator when its condition no longer holds).
#DocsGates: close({
	// site_links — every generated page link resolves in the built site.
	site_links?:    *true | bool @go(SiteLinks)
	// sidebar_links — every sidebar entry resolves on its page.
	sidebar_links?: *true | bool @go(SidebarLinks)
	// prune — stale generated pages (projections without a toggle) are removed.
	prune?:         *true | bool
})
