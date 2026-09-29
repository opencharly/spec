package spec

// scan_seams.go — the host-coupled legs the candy-scan fetch fix-point (loaderkit.ScanCandyFromLocal)
// reaches. Relocated from sdk/loaderkit/scan_orchestrate.go (#55 C3b-ii) so it can be the parameter
// type on the spec.ProjectLoader.ScanCandyFromLocal seam method — the interface lives in this
// dedicated spec module, so its param types must live here too. The caller (charly's scanSeamsFor,
// candy/plugin-build's scanSeamsLeg) builds these as closures capturing its config/opts + host
// mechanisms (registry, refs backend); the pure fix-point in loaderkit never inspects a
// package-main type. loaderkit keeps a `type ScanSeams = spec.ScanSeams` forwarder (mirroring its
// RemoteDownload alias) so its own signature + candy/plugin-build's call sites stay terse.
type ScanSeams struct {
	// CollectRemoteRefs runs the reachability-scoped remote-ref walk over the project's boxes +
	// this local candy set, returning each distinct (repo, git-tag) to fetch. Host closure:
	// CollectRemoteRefsOpts(cfg, FinalizeScannedCandies(localScanned, nil), WithLocalRawRefs(opts, localScanned)).
	CollectRemoteRefs func(localScanned map[string]ScannedCandy) ([]RemoteDownload, error)
	// EnsureRepo resolves a (repoPath, version) to a local cache directory, fetching + auto-migrating
	// on a cache miss (host closure: EnsureRepoDownloaded).
	EnsureRepo func(repoPath, version string) (string, error)
	// ScanRemote scans the wanted bare refs out of a downloaded repo cache dir (host closure:
	// requireCandyScanner().ScanRemoteCandy(cacheDir, repoPath, wantRefs, parseCandyYAML)).
	ScanRemote func(cacheDir, repoPath string, wantRefs map[string]bool) (map[string]ScannedCandy, error)

	// Diag receives the scan's own diagnostics — candy-version skew and local-shadow notes —
	// as DATA, instead of them going straight to stderr.
	//
	// They USED to be `fmt.Fprintf(os.Stderr, ...)` calls inside the arbiter, which made them
	// unstructured and therefore uncountable: `charly box validate` could not report how many
	// warnings a run produced, because they never reached its diagnostics. A summary that
	// cannot see them can only omit the number or state a false one.
	//
	// ONE path carries every scan diagnostic, and the LEVEL is the first argument: a resolvable
	// multi-tag set is NOT a defect (the arbiter picks the newest REFERENCED tag and resolution
	// succeeds), so it is reported at DiagInfo and must not gate; only a genuinely unresolvable
	// set is DiagWarning. Keeping the level ON this sink — rather than adding a second
	// warning-only channel — is what makes "count the warnings" and "report the info" the same
	// consumer-side reduction, with no duplicated formatter to drift.
	//
	// Optional. When nil the diagnostics still go to stderr exactly as before (prefixed
	// "Warning:" or "Notice:" by level), so every existing caller keeps its current behaviour
	// and this stays a purely additive seam.
	Diag func(level DiagLevel, format string, args ...any)
}

// DiagLevel is the disposition of one scan diagnostic. It is the LEVEL, not the emitter, that
// decides whether a consumer counts the line as a gate-failing warning.
type DiagLevel string

const (
	// DiagInfo is a RESOLVABLE observation: the arbiter had a deterministic winner, so the
	// dependency resolved and nothing needs fixing to proceed. It is reported so the skew stays
	// visible and fixable, but a consumer MUST NOT count it as a warning — R10's zero-warnings
	// gate is for unresolved defects, and a successful arbitration is not one.
	DiagInfo DiagLevel = "info"
	// DiagWarning is a genuinely UNRESOLVABLE condition (no candidate, or no determinable
	// winner). It is the only scan diagnostic that may fail a gate.
	DiagWarning DiagLevel = "warning"
)
