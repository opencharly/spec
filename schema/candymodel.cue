package schema

// CUE schema for the serializable candy BUILD model (S-CM, Collection A). #CandyModel is the
// wire form of a runtime candy's build model — the plan steps + resolved package/service/env/route
// sections a plugin (validate, check-include, the K3-D deploy-plan compiler) reads WITHOUT holding
// the live *Candy. It is DISTINCT from #CandyView (identity + dep-graph only, resolvedproject.cue):
// #CandyView is what inspect/list/status read; #CandyModel is what validate + the plan-include
// splicer + K3-D read. The host projects the runtime *Candy into it (charly/resolved_project_host.go
// projectCandyModel); a plugin reconstructs it off the resolved-project envelope's CandyModels map.
//
// The four package/env/route sub-shapes (#PackageSection/#TagPkgConfig/#EnvConfig/#RouteConfig) were
// hand structs in sdk/deploykit (layer_model.go) + sdk/kit (env.go); they are CUE-sourced here and
// those homes alias onto spec (SDD: wire types are CUE-sourced). Composed sub-defs #Step/#Op/
// #CandyService/#CandyExtract/#CandyData/#CandyVolume/#CandyAlias/#Shell/#EnvDependency/#CandyApk
// come from 00_common.cue / candy.cue. `package schema`; concatenated into the spec compilation unit.
// S-CM spike PROVEN live (scratchpad/k5-scm-spike-verdict.md + k5a-buildvocab-spike-verdict.md).

// #PackageSection — a generic format-specific package section (rpm/deb/pac/aur). Raw carries the
// full YAML map for template rendering. Mirrors deploykit.PackageSection (which now aliases this).
#PackageSection: {
	format_name?: string @go(FormatName)
	packages?: [...string] @go(Packages)
	raw?: {...} @go(Raw,type=map[string]any)
}

// #TagPkgConfig — a distro/version-specific package section (debian:13, ubuntu:24.04, fedora:43…).
// Raw captures the full YAML so tag sections carry repos:/options:/keys:. Mirrors deploykit.TagPkgConfig.
#TagPkgConfig: {
	package?: [...string] @go(Package)
	raw?: {...} @go(Raw,type=map[string]any)
}

// #EnvConfig — resolved candy env (KEY=value vars + PATH-append entries). Mirrors kit.EnvConfig.
#EnvConfig: {
	vars?: {[string]: string} @go(Vars)
	path_append?: [...string] @go(PathAppend)
}

// #RouteConfig — a resolved route declaration (host + port-as-string). Mirrors deploykit.RouteConfig.
// Port is a STRING here (the resolved form), distinct from #CandyRoute.port (authored int).
#RouteConfig: {
	host?: string @go(Host)
	port?: string @go(Port)
}

// #CandyModel — the serializable candy build model. Every field optional (a projection). name is the
// stable key. Field order mirrors the runtime *Candy accessors (deploykit.CandyModel interface).
#CandyModel: {
	// --- identity ---
	name?:       string @go(Name)
	version?:    string @go(Version)
	source_dir?: string @go(SourceDir)
	reboot?:     bool   @go(Reboot)

	// --- host-precomputed predicates (#67 render-DRIVE move) ---
	// has_content / has_install_files are the LIVE *Candy.HasContent() / HasInstallFiles()
	// verdicts, computed HOST-SIDE (the live Candy has the env/ports/route/volumes/aliases/
	// libvirt/init fields + the fs-probe caches the envelope CandyModel cannot recompute
	// faithfully) and carried here so the specCandyAdapter matches the live *Candy
	// byte-exactly — the candy-graph composition (ExpandCandy/ResolveCandyOrder) and the
	// pixi-bound intermediate detection both gate on these. has_content does NOT count plan
	// steps, so a pure-composition candy (e.g. agent-forwarding: candy:
	// [gnupg,direnv,ssh-client], only a check plan) has has_content=false. Such a candy is
	// still admitted to the candy graph when it carries a PLAN, so that plan bakes into the
	// ai.opencharly.description label (ADE: every candy's plan is runnable acceptance); it
	// contributes nothing to the build, since has_content=false implies empty RunOps. A
	// content-free composing candy with no plan is skipped, as a pure grouping node.
	// Admission is deliberately NOT keyed to the description: ADE makes a description
	// mandatory on every candy, so that would admit every composing candy unconditionally.
	// The admission predicate is deploykit's layerEntersOrder.
	has_content?:        bool @go(HasContent)
	has_install_files?: bool @go(HasInstallFiles)

	// --- plan + lowered ops ---
	plan?: [...#Step] @go(Plan)
	run_ops?: [...#Op] @go(RunOps)

	// --- services / extract / data / apk ---
	// service — the candy's service declarations as OPAQUE BYTES, on the hook/extract/data pattern:
	// the authored #CandyService stays in spec (it is #Candy's own field), and the IR carries the
	// JSON bytes rather than importing it, so the def can move to the plugin that owns the body.
	// The consumer side is TWO repos: the sdk adapter decodes, and plugin-box decodes its four
	// model sites. PROGRAM/schema-ownership-rca-rdd.md section 7c carries the per-field inventory.
	service?: bytes @go(Service,type=RawBody)
	// extract — the candy's file extractions as OPAQUE BYTES, the same shape as hook above: the
	// authored #CandyExtract stays in spec (it is #Candy's own field), and the IR carries the JSON
	// bytes rather than importing the shape, so the def can move to the plugin that owns the body.
	extract?: bytes @go(Extract,type=RawBody)
	// data — the candy's data-seeding entries as OPAQUE BYTES, the third field on the hook/extract
	// pattern: the authored #CandyData stays in spec (it is #Candy's own field), and the IR carries
	// the JSON bytes rather than importing it, so the def can move to the plugin that owns the body.
	data?: bytes @go(Data,type=RawBody)
	apk?: [...#CandyApk] @go(Apk,type=[]ApkPackageSpec)

	// --- package surface (resolved) ---
	top_packages?: [...string] @go(TopPackages)
	// packaging — the candy's `packaging:` section (the single source of truth
	// for distro package metadata + variants). Carried on the build model so the
	// specCandyAdapter can implement CandyReader.Packaging() — the deploy-plan
	// compiler reads it to emit a LocalPkgInstallStep (PackageName = packaging.name).
	packaging?: #Packaging @go(Packaging,optional=nillable)
	format_sections?: {[string]: #PackageSection} @go(FormatSections)
	tag_sections?: {[string]: #TagPkgConfig} @go(TagSections)

	// --- env / vars / route / shell ---
	vars?: {[string]: string} @go(Vars)
	env?:   #EnvConfig   @go(Env,optional=nillable)
	route?: #RouteConfig @go(Route,optional=nillable)
	shell?: #Shell       @go(Shell,optional=nillable)

	// --- volumes / aliases ---
	// volumes / aliases — OPAQUE BYTES for the same reason: the build model projects what the
	// consumers need without importing the authored shapes (I1/D-SCH-7).
	volumes?: bytes @go(Volumes,type=RawBody)
	aliases?: bytes @go(Aliases,type=RawBody)

	// --- security / hooks / skills (the OCI-label-collector surface: CollectSecurity/CollectHooks
	// read this per-candy field then apply the box-level #ResolvedBoxView.security override) ---
	security?: #Security  @go(Security,optional=nillable)
	// hook — the candy lifecycle hooks as OPAQUE BYTES. The authored #CandyHook STAYS in spec (it is
	// #Candy's own field and both metadata shapes reference it); what changes is that the IR stops
	// importing it, so the build engine reads bytes and deploykit's specCandyAdapter decodes them.
	hook?: bytes @go(Hook,type=RawBody)
	// skills: the NAMES of the `skill:` nodes this candy OWNS — the build-time association
	// `skill.cue`'s `owner:` records, projected onto the wire model so a consumer holding only
	// the model can answer "which skills does this candy carry?" without reaching the authored
	// nodes (Config.PluginKinds["skill"], opaque bodies keyed by node name).
	//
	// WHY IT BELONGS HERE. The model exists so a plugin can read a candy's build contract WITHOUT
	// holding the live *Candy (opencharly/spec#254). `charly box validate` reads it: a candy whose
	// only content is the skill nodes it owns has install content, because kind:skill installs
	// their bodies into every image that composes it (ops.OpContent, opencharly/sdk#598). Which
	// IMAGES carry a skill is kind:skill's own per-box question, answered from the nodes, not from
	// here. Owner-qualified NAMES, not opaque bodies: the bodies travel through
	// Config.PluginKinds and the marketplace generator, and duplicating them here would create a
	// second source for content this model does not resolve.
	skills?: [...string] @go(Skills)

	// --- validate read-surface (candy-local config the validate ENGINE checks) ---
	// external_builder: the reserved word of an EXTERNAL builder plugin this candy selects
	// (from the candy manifest external_builder:). validateCandyContents reads it to accept a
	// candy whose only content is an external-builder selection (deploykit EmitExternalBuilderStages).
	external_builder?: string @go(ExternalBuilder)
	// (the candy-level `libvirt:` field was removed here alongside #Candy — see
	// candy.cue's note; CollectLibvirtSnippets, its only reader, was already dead.)
	engine?:  string @go(Engine)
	port_relay?: [...int] @go(PortRelayPorts,type=[]int)
	service_files?: [...string] @go(ServiceFiles)
	env_require?: [...#EnvDependency] @go(EnvRequire)
	env_accept?: [...#EnvDependency] @go(EnvAccept)
	secret_require?: [...#EnvDependency] @go(SecretRequire)
	secret_accept?: [...#EnvDependency] @go(SecretAccept)
	mcp_require?: [...#EnvDependency] @go(MCPRequire)
	mcp_accept?: [...#EnvDependency] @go(MCPAccept)

	// --- W9 mass-edit interface-completeness fill: the remaining fields the 42-file
	// CandyReader repoint needs, already authored on #Candy (candy.cue) but not yet
	// projected onto the build model. artifact/requires_capability/secret/port mirror
	// candy.cue's own fields+shapes exactly; capability widens the CandyView's narrow
	// (preserve_user-only) #CandyCapabilitiesView to the full authored #CandyCapability.
	// artifact — OPAQUE BYTES; the authored #CandyArtifact stays #Candy's, the model carries bytes.
	artifact?: bytes @go(Artifact,type=RawBody)
	requires_capability?: [...(string & !="")] @go(RequiresCapability)
	// need/use mirror candy.cue's authored pair (§8; schema/capability.cue carries the shapes),
	// projected onto the build model the same way every other candy field is.
	need?: #CandyNeed @go(Need,optional=nillable)
	use?:  #CandyUse @go(Use,optional=nillable)
	capability?: #CandyCapability @go(Capability,optional=nillable)
	secret?: [...#CandySecret] @go(Secret)
	// port mirrors candy.cue's own union shape exactly (a plain int OR a "proto:port" string,
	// normalized Go-side to PortSpec — there is no reusable #PortSpec def, per candy.cue's own note).
	port?: [...(int & >0 & <=65535 | string & =~"^[a-z+-]+:[0-9]+$")] @go(Port,type=[]PortSpec)
	// bake_plugin: the FINAL bare-string form (post remote-sibling qualification — see
	// spec.CandyRefs, the hand-written pipeline-state type carrying the rich pre-qualification
	// form). generate.go's emitBakedPlugins reads this to COPY the plugin's pre-built provider
	// binary into every composing image.
	bake_plugin?: [...#CandyRef] @go(BakePlugin)
}
