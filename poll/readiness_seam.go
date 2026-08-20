package poll

// readiness_seam.go — the executor-side readiness SEAM var, homed in the spec/poll
// slice so both the host/guest executors (spec/exec) and every other reader reach
// the SAME live var. The host-side SSHExecutor's wait-for-SSH bounds come from the
// project's defaults.readiness, which only the charly HOST can LoadUnified. Rather
// than the executor self-loading the project (a loader coupling), the host injects
// its project-aware resolver into ReadinessProvider at init (charly/readiness_config.go);
// a standalone consumer falls back to the built-in defaults (ResolveReadiness(nil) —
// always safe + never-hang).
//
// This is a LIVE seam var charly WRITES, so it is homed ONCE here and every reader +
// the charly writer references spec/poll.ReadinessProvider DIRECTLY — a `var X =
// poll.ReadinessProvider` copy-alias is FORBIDDEN (it snapshots the func value at
// init and breaks write-through).

// ReadinessProvider returns the resolved readiness bounds the executors' waits use.
// Defaults to the built-in bounds; charly overrides it at init with its project-aware
// loadedReadiness (charly/readiness_config.go).
var ReadinessProvider = func() ResolvedReadiness {
	rr, _ := ResolveReadiness(nil)
	return rr
}
