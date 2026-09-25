// version.cue — the HEAD schema version + the migration floor: the SINGLE source
// of truth for schema versioning. `charly task cue-gen` reads these via the CUE API and
// emits spec/version_gen.go (const SchemaVersion / SchemaFloor); the
// parsed HEAD lives in kit.LatestSchemaVersion() / kit.SchemaFloor(), which
// parse those generated consts. All three defs are @go(-) so `cue exp
// gengotypes` emits no stray Go type (the #CalVer pattern).
//
// The literals are validated by #CanonCalVer — a STRICT fixed-width YYYY.DDD.HHMM
// (4/3/4 digits), tighter than the loose #CalVer wire pattern in _common.cue — so
// a non-canonical literal fails schema COMPILATION here (fail-fast at gen time),
// matching exactly what the strict Go kit.ParseCalVer accepts. kit.MustCalVer at
// process start is the belt-and-braces Go-side backstop.

// A canonical, fixed-width CalVer: 4-digit year, 3-digit day-of-year, 4-digit HHMM.
#CanonCalVer: string & =~"^[0-9]{4}\\.[0-9]{3}\\.[0-9]{4}$" @go(-)

// #SchemaVersion is the HEAD schema CalVer — the version every current-format
// config is stamped to and the value the load-time gate requires. Bumped by the
// schema-compaction cutover (compact node grammar: collections inline in the kind
// value, steps as an unnamed `plan:` list, plugin-verb sugar replacing the
// plugin:/plugin_input: envelope, live-verb fields relocated to plugin input
// defs, box env as a map) — a format change on every authored wire surface,
// migrated by the single `apply:` reshaper hook in candy/plugin-migrate. Then
// bumped again by the candy-level `libvirt:` field removal (Cutover B unit
// 3+4, R5 claim-keyed sweep): the field had zero live Go consumers, migrated
// away by the `stripCandyLibvirtField` reshaper hook. Bumped again (K5-B/
// validation-correctness batch) by the deploy-scope `shell:` overlay field
// removal: #Deploy.shell (#DeployShellOverlay) was authorable but had ZERO
// live consumer — MergeDeployShell, its only would-be merge, never had a
// production call site anywhere in this repo's history — migrated away by
// the `stripDeployShellOverlay` reshaper hook. Bumped again by the
// k8s→kubernetes full naming cleanup (the deploy substrate kind + cluster-template
// kind + every #K8s* def + the derived target enum): an authored WIRE-key change
// (`k8s:` → `kubernetes:`, and the inner deploy-knobs block `kubernetes:` → `deploy:`),
// migrated by the two rename_key ops in candy/plugin-migrate/migrations.cue.
// Bumped again by the packaging-section cutover (the candy `localpkg:` map →
// `packaging?: #Packaging` section, the `local_pkg` source-build field removal,
// and the LocalPkgInstallStep IR → the download leg): an authored WIRE-key
// change on the candy surface (`localpkg:` → `packaging:`) + the deploy
// InstallStepView wire (pkgbuild_ref/project_dir → package_name/version),
// migrated by the remove_key/rename_key ops in candy/plugin-migrate/migrations.cue
// when the spec submodule pointer is bumped (step 3 of the cross-repo cutover).
// Bumped again by the install_template removal (the strict-cleanup cutover):
// the legacy top-level `#Format.install_template` / `#Builder.install_template`
// fields (the (install, container) fallback the resolvers fell back to when
// `phase:` lacked the cell) are REMOVED — their content migrates into
// `format.<fmt>.phase.install.container`, the phase: block's single source of
// truth. Migrated by a structural-reshape Go hook (the nested move
// format.<fmt>.install_template → format.<fmt>.phase.install.container cannot
// be expressed as rename_key/move_key ops — see the charly-side migration
// entry) when the spec submodule pointer is bumped (step 3 of the cross-repo
// cutover).
// Bumped again by the GPU-configuration-surface cutover: #LibvirtGraphics.gl
// changes SHAPE, from a bare scalar (`gl: "yes"`, which could only ever reach
// spice's enable= attribute) to #LibvirtGraphicsGL, so that rendernode= — the
// attribute that points virtio-gpu at a specific host DRM node — is expressible
// at all. An authored WIRE-SHAPE change on the vm surface, migrated by the
// `reshapeGraphicsGL` reshaper hook in candy/plugin-migrate: the scalar sits at
// vm.libvirt.devices.graphics[].gl, a field inside a LIST element, which none of
// the four key-transform ops can reach.
// Bumped again by the run-scoped-instruments cutover (Cutover A, task 1):
// NEW authored wire surface — `instrument?:` on every deployable
// substrate-node body (deploy.cue, beside disposable:), each entry carrying
// id/phase/pipeline + the plugin-verb sugar pair, and the machine-written
// evidence-manifest contract (#EvidenceRow → .check/<bed>/<calver>/evidence.yml
// with segment/artifact/pipeline rows). An authored WIRE-key change (new fields
// open on the deploy surface + a new evidence-file format), migrated by the
// companion `record:`-harvest cutover entry in candy/plugin-migrate.
// Bumped again by the group-kind removal cutover (Cutover C task 1): `group`
// is removed from #ResourceKind (the targetless deploy-group node kind dies —
// pod/vm/kubernetes/local/android remain; #105 removed the schema arm and
// Deploy.IsGroup, this bump widens the migratable window for the consumer
// half). Authored `group:` deploy nodes are rewritten by the
// `unrollGroupDeploy` reshaper hook in candy/plugin-migrate (the FIRST member
// becomes the deploy primary and inherits the group scalars —
// disposable/lifecycle/description — the REMAINING members become deploy-level
// siblings, nested groups rewrite recursively). The migration-table row needs
// a head above the previous one: the table is strictly ascending within
// [Floor, Head] and the previous row (record-field-to-instrument) already sat
// AT 2026.248.1030, so a residual-config rewrite is impossible without this
// bump.
// Bumped again by the per-deploy VM-shape override cutover: `#Deploy`'s dead
// VM-shape `cpu`/`ram` fields are given meaning by a reader (candy/plugin-vm's
// hostConfigResolve, chain-inheriting over `from:`) that already landed as
// plugin-vm#39 and reads these spellings once plugin-vm bumps its spec pin to
// the tag this leg produces; the cpu field is
// corrected from the outlier plural spelling `cpus:` to the singular `cpu:`,
// matching `#Vm` (the template it overrides) exactly. The unreadable `disk_size`
// field and the never-implemented `variants:`/`#VmVariant` surface are DELETED
// rather than parked — the cpu/ram/disk_size fields were authorable-but-inert
// since the initial spec import (`11dcd6d`), while `variants:`/`#VmVariant` was
// added in #86 and never implemented. All of them were DEAD (zero readers, zero
// authors), so nothing authored needs migrating; the old `cpus:` key is simply
// gone. NO migration-table entry:
// a rename would have to be scoped `under_kind: vm` and the op-walker's
// under_kind marks every mapping nested within the entity, which would also
// rewrite the LIVE `security: {cpus: "2.5"}` string quota into a schema-invalid
// `security: {cpu: …}`. Re-adding under the correct spelling sidesteps that.
// Re-stamped to the merge-time CalVer by the fresh pr-validator.
//
// Bumped again by the kubevirt-substrate cutover: a NEW authored WIRE surface —
// the 6th deploy substrate `kubevirt:` (#ResourceKind gains the word; #KubeVirt +
// #KubevirtSource + the GPU/firmware/network/instancetype blocks are new), a new
// `kubevirt:` sub-block on `#Kubernetes`, and the new machine-written
// `kubevirt_state:` deploy field. Purely ADDITIVE (no existing authored key changes
// shape), so nothing authored needs migrating; the bump widens the migratable
// window per policy. No migration-table entry (the surface is new, not renamed).
#SchemaVersion: #CanonCalVer & "2026.261.1747" @go(-)

// #SchemaFloor is the OLDEST schema version `charly migrate` can migrate FROM. At
// the migration-baseline reset it EQUALS #SchemaVersion — the deleted 47-step chain
// was the only path from any older format, so nothing below the current HEAD is
// migratable. An ordinary future cutover bumps ONLY #SchemaVersion, widening the
// [floor, HEAD) migratable window; a future baseline reset raises the floor again.
#SchemaFloor: #CanonCalVer & "2026.174.1100" @go(-)
