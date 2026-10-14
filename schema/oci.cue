// CUE schema for the OCI-plugin wire types — the host<->plugin envelopes the OCI
// plugin (registry.go + merge.go logic; go-containerregistry lives in the candy, not
// core) serves. Package-less; concatenated into the spec compilation unit. NOT
// authoring kinds (never in #Node/#Op) — pure host<->plugin wire structs, single-
// sourced here so `charly task cue-gen` produces the Go structs. @go names match the Go
// field names; JSON tags drive the marshalled wire envelope.

// #MergeRequest — the host resolves the box (image ref + merge limits + engine) and
// hands the OCI plugin everything it needs to run the go-containerregistry layer
// merge, so the plugin never imports charly core config-loading.
#MergeRequest: {
	image_ref:     string @go(ImageRef)
	max_mb:        int    @go(MaxMB,type=int)
	max_total_mb:  int    @go(MaxTotalMB,type=int)
	engine:        string @go(Engine)
	dry_run?:      bool   @go(DryRun)
}

// #MergeReply — the OCI plugin's merge outcome. layers_before/layers_after report the
// layer-count reduction; skipped is true when the image was too large or had nothing
// to merge; error carries a per-merge failure (the reply-error convention — e.g. the
// known podman-load EEXIST case — distinct from an infra Go error on Invoke); notes
// carries the human progress lines the host prints.
#MergeReply: {
	layers_before?: int    @go(LayersBefore,type=int)
	layers_after?:  int    @go(LayersAfter,type=int)
	skipped?:       bool   @go(Skipped)
	error?:         string @go(Error)
	notes?: [...string] @go(Notes)
}

// #ImageUserInput — inspect a remote image's /etc/passwd for the user with the given
// uid (the build engine's base-image adopt-user probe).
#ImageUserInput: {
	ref: string @go(Ref)
	uid: int    @go(UID,type=int)
}

// #UserInfo — one /etc/passwd entry (name:uid:gid:...:home). Empty reply (found=false)
// when no user with the requested uid exists in the image.
#UserInfo: {
	found: bool   @go(Found)
	name?: string @go(Name)
	uid?:  int    @go(UID,type=int)
	gid?:  int    @go(GID,type=int)
	home?: string @go(Home)
}

// #CacheTransferRequest — the verb:oci cache-push / cache-pull wire input: a
// named spec/cache ArtifactStore's OCI Image Layout directory and the registry
// reference it moves to/from. Single-sourced here (R3) so candy/plugin-oci's
// transport legs and candy/plugin-cache's `charly cache push/pull` leaves share
// ONE decoded type instead of two hand-written copies that can drift.
#CacheTransferRequest: {
	dir:      string @go(Dir)
	ref:      string @go(Ref)
	insecure?: bool   @go(Insecure)
}

// #CacheTransferReply — the transport result: the resolved digest and the number
// of entry manifests (cache keys) moved, plus the canonicalised ref.
#CacheTransferReply: {
	digest?:  string @go(Digest)
	entries?: int    @go(Entries,type=int)
	ref?:     string @go(Ref)
}

// #PluginArtifactTransferRequest — the verb:oci plugin-push / plugin-pull wire
// input: the OCI Image Layout directory the plugin artifact is emitted to or
// materialized in, and the registry reference it moves to/from. ONE pair for both
// legs, exactly the #CacheTransferRequest/#CacheTransferReply precedent (R3):
// plugin-oci's transport legs and their consumers decode ONE type instead of two
// hand-written copies that can drift.
//
// The transport is REF-AGNOSTIC: `ref` is any distribution-spec reference, so it
// also addresses a registry charly itself serves — nothing here assumes who runs it.
// `platform` selects which per-platform manifest to emit or consume; empty means the
// runtime's own GOOS/GOARCH. A request naming a platform the artifact does not carry
// is an ERROR naming what WAS available (#PluginArtifactTransferReply.platforms) —
// never a fall-through to whichever platform happens to be there.
#PluginArtifactTransferRequest: {
	dir:       string @go(Dir)
	ref:       string @go(Ref)
	insecure?: bool   @go(Insecure)
	platform?: string @go(Platform)
}

// #PluginArtifactTransferReply — the transport result: the resolved digest of what
// was moved (the index for a push, the SELECTED per-platform manifest for a pull —
// the digest the consumer verifies and keys its store by, D-STORE-3/4), the platform
// actually selected, the canonicalised ref, and every platform the artifact
// advertises, so a caller that could not be served can name the alternatives.
#PluginArtifactTransferReply: {
	digest?:    string     @go(Digest)
	ref?:       string     @go(Ref)
	platform?:  string     @go(Platform)
	platforms?: [...string] @go(Platforms)
}

// #PluginArtifactMetadata — the CONTENT of a plugin artifact's `config` blob (the
// image-spec artifact decision-tree case 3: content blobs + JSON configuration), i.e.
// everything a consumer must know to decide whether the pulled binary may run here.
//
// It is the D-STORE-4 key, spelled out: "a plugin binary's key covers its module source,
// go.sum, the toolchain and the platform". The store keys on it, so two builds that differ
// in ANY of those are two entries, never one entry that silently serves the wrong bytes.
//
// VERIFY HARD (a mismatch is a hard error before the binary is served):
//   - platform.os/architecture/variant — restated from the index descriptor the consumer
//     SELECTED. The two must agree: a manifest whose config claims a different platform
//     than the descriptor that pointed at it is an integrity failure, not a redundancy;
//   - libc — a glibc binary does not run on a musl host, and image-spec's `platform`
//     object CANNOT express libc. That gap is the whole reason this field exists here;
//   - protocol — the go-plugin transport family (spec/transport.Handshake.ProtocolVersion).
//     Declared so a mismatch fails HERE, clearly, instead of as a handshake refusal; the
//     value must come from the ONE constant, never a transcribed copy.
//
// RECORD AND REPORT (never hard-gated): calver, schema_digest, revision, toolchain. The
// authoritative wireframe check is the CUE schema the binary serves over Describe — spliced
// and gated by the host — so a stale calver whose schema still splices is harmless, and
// gating on it would refuse working plugins. Reporting it is what makes the skew visible.
#PluginArtifactMetadata: close({
	// module: the provider's Go module path (e.g. github.com/opencharly/plugin-bpf/candy/plugin-bpf).
	module: string
	// revision: the source-control revision the binary was built from.
	revision: string
	// go_sum: the digest of the module's go.sum at build time (D-STORE-4's second component).
	go_sum: string
	// toolchain: the Go toolchain that produced the binary (e.g. go1.26.0).
	toolchain: string
	// os / architecture / variant: the build's own GOOS/GOARCH(/GOAMD64|GOARM64) identity,
	// in the image-spec `platform` vocabulary the index descriptor also uses.
	os:           string
	architecture: string
	variant?:     string
	// libc: the C library the binary links against. image-spec cannot express it.
	libc: "glibc" | "musl"
	// protocol: the go-plugin transport family the binary speaks.
	protocol: int
	// calver: the charly release the plugin was built against (reported, not gated).
	calver: string
	// schema_digest: sha256 of the CUE schema the binary serves over Describe — the
	// wireframe identity, so a consumer can say WHICH schema it is about to splice.
	schema_digest: string
	// providers: the "<class>:<word>[:<parent>]" words this binary serves — the same set
	// its `.providers` layer carries and its Describe advertises. Three copies of one fact
	// on purpose: the layer is what makes a source-less PRESCAN possible, this is what the
	// consumer can check before connecting, and Describe is the authority afterwards.
	providers: [...#PluginCapability]
	// layers: layer ROLE → the descriptor digest the manifest carries for it. Roles are
	// "binary" | "manifest" | "providers" (the media types below identify the same three;
	// the map lets a consumer assert "the binary blob I verified is the one this metadata
	// names" without re-deriving which layer is which).
	layers: {[string]: string}
})

// #ContainerDiskEmitRequest — the containerDisk emit wire input this pair was
// authored for: a materialized guest disk, the in-layer path it is stored at, the
// OCI config labels to carry, and the registry reference to push.
//
// STATUS (corrected 2026-10-14): this pair has NO consumer on `main` — it is
// ORPHANED. The emit engine that actually shipped is sdk/deploykit/vm_box_emit.go
// (EmitVmBox / EmitVmBoxAt), driven from plugin-vm/candy/plugin-vm/vm_box_emit.go by
// the merged opencharly/plugin-vm#50. The verb:oci `container-disk-emit` leg these
// types name was authored in opencharly/plugin-oci#5, which is CLOSED, not merged,
// and no `charly vm box publish` command exists. Nothing decodes these types; their
// removal is opencharly/spec#231 (deliberately NOT part of this PR).
// disk_path is the host path of the disk; in_image_path defaults to the KubeVirt
// containerDisk contract /disk/disk.img; layout_dir optionally also writes a
// local OCI Image Layout (oci:<dir>).
#ContainerDiskEmitRequest: {
	disk_path:      string @go(DiskPath)
	in_image_path?: string @go(InImagePath)
	labels?:        {[string]: string} @go(Labels,type=map[string]string)
	ref:            string @go(Ref)
	insecure?:      bool   @go(Insecure)
	layout_dir?:    string @go(LayoutDir)
}

// #ContainerDiskEmitReply — the emit result: the pushed digest, the media type
// actually written (the caller asserts the +gzip containerDisk contract), the
// layer byte size, and the layout dir when one was requested.
#ContainerDiskEmitReply: {
	ref?:        string @go(Ref)
	digest?:     string @go(Digest)
	media_type?: string @go(MediaType)
	layer_size?: int    @go(LayerSize,type=int64)
	layout_dir?: string @go(LayoutDir)
}
