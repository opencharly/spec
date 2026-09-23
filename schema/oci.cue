// CUE schema for the OCI-plugin wire types — the host<->plugin envelopes the OCI
// plugin (registry.go + merge.go logic; go-containerregistry lives in the candy, not
// core) serves. Package-less; concatenated into the spec compilation unit. NOT
// authoring kinds (never in #Node/#Op) — pure host<->plugin wire structs, single-
// sourced here so `task cue:gen` produces the Go structs. @go names match the Go
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
