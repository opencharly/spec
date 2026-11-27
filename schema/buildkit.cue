package schema

// #Buildkit — the buildkitd daemon settings a project authors under `defaults.buildkit:`.
//
// The daemon is PROJECT-wide: one buildkitd serves every box a build composes, so the
// drive reads this from `defaults.buildkit` (a box-level value is documented as ignored
// rather than silently merged).
//
// Every field is optional. An empty value selects the daemon's own default, EXCEPT the GC
// budget: the drive always writes an explicit budget (100 GB by default), because
// buildkitd's compiled default GC policy keeps a 373 GB free-space target and, on a host
// with less free space, prunes reclaimable cache down to its ~10 GB reserve floor on every
// pass — which makes a repeated build of an unchanged tree re-execute its layers
// (opencharly/charly#1292). Values accept buildkitd's own spellings ("100GB", "30%",
// a byte count) verbatim.
#Buildkit: {
	// process_mode is the OCI worker's process mode ("sandbox" by default).
	process_mode?: string @go(ProcessMode)
	// max_parallelism bounds the stages of ONE build that run concurrently
	// (`[worker.oci] max-parallelism`). 0/absent = the daemon's own default; the
	// drive resolves `defaults.stage_parallelism` in when this is absent.
	max_parallelism?: int & >=0 @go(MaxParallelism)
	// snapshotter pins the worker's snapshotter ("overlayfs" or "native"); empty =
	// the daemon's own "auto" resolution.
	snapshotter?: string @go(Snapshotter)
	// no_process_sandbox runs subprocesses in the main pid namespace (needed when
	// buildkitd itself runs inside a container).
	no_process_sandbox?: bool @go(NoProcessSandbox)
	// gc enables the worker's garbage collector. Absent = enabled with the budget.
	gc?: bool @go(GC,type=*bool)
	// reserved_space is the cache floor the GC never reclaims below.
	reserved_space?: string @go(ReservedSpace)
	// max_used_space is the cache ceiling the GC reclaims above.
	max_used_space?: string @go(MaxUsedSpace)
	// min_free_space is the free-space target the GC tries to leave. Keep it SMALL:
	// a large target is what prunes the cache on a host that cannot meet it.
	min_free_space?: string @go(MinFreeSpace)
	// gcpolicy renders `[[worker.oci.gcpolicy]]` entries (advanced per-record policies).
	gcpolicy?: [...#BuildkitGCPolicy] @go(GCPolicy)
	// registry_mirror is the local charly OCI registry the daemon mirrors ghcr.io to.
	// Absent = the drive's env fallback (CHARLY_BUILD_REGISTRY_MIRROR).
	registry_mirror?: string @go(RegistryMirror)
	// extra is raw buildkitd.toml appended verbatim after the managed sections — the
	// escape hatch for a daemon setting charly does not model.
	extra?: string @go(Extra)
}

// #BuildkitGCPolicy — one `[[worker.oci.gcpolicy]]` entry: a per-record override of the
// worker's cache budget. Empty fields are omitted; `all` applies it to every record.
#BuildkitGCPolicy: {
	reserved_space?: string @go(ReservedSpace)
	max_used_space?: string @go(MaxUsedSpace)
	min_free_space?: string @go(MinFreeSpace)
	keep_duration?:  string @go(KeepDuration)
	all?:            bool   @go(All)
	filters?:        [...string] @go(Filters)
}
