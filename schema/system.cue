// system.cue — the `system:` kind: the per-host LOCAL SYSTEM INFO.
//
// The host's identity snapshot (hostname, distro, kernel, arch, GPU, …) lives in
// the per-host charly.yml (~/.config/charly/charly.yml) under `system:`, so a
// command can answer "what is this host?" from the unified local config instead
// of re-probing the host on every invocation. Populated by `charly doctor` /
// `charly status`; read by any command that needs the host identity.

// #SystemInfo is the top-level `system:` block — the host identity snapshot.
#SystemInfo: {
	hostname?: string @go(Hostname)
	// distro_id is the host distro identifier (e.g. "fedora", "arch", "debian").
	distro_id?: string @go(DistroID)
	// distro_version is the host distro version string (e.g. "43", "rolling").
	distro_version?: string @go(DistroVersion)
	// kernel is the running kernel release (uname -r).
	kernel?: string @go(Kernel)
	// arch is the host architecture (uname -m).
	arch?: string @go(Arch)
	// gpu is the primary GPU description (vendor + model), when detectable.
	gpu?: string @go(GPU)
	// virtualization is the detected virtualization backend (kvm, qemu, none).
	virtualization?: string @go(Virtualization)
	// podman is the podman version string, when present.
	podman?: string @go(Podman)
	// updated_at is when this snapshot was last refreshed (RFC3339).
	updated_at?: string @go(UpdatedAt)
}
