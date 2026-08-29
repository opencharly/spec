// distro_vocab.cue — the SINGLE source for the distro id space and each id's traits.
//
// Everything that used to be a hand-written Go table or a hardcoded switch on a distro
// name derives from here via `task cue:gen`:
//
//   #Distros  →  spec.DistroFormats   (id → package format)   replaces hostenv.distroIDToFormat
//             →  spec.DistroSSHUnits  (id → OpenSSH service)  replaces vmshared.sshUnitForDistro
//             →  spec.DistroInits     (id → init system)      replaces `if distro == "alpine"`
//             →  spec.DistroIDs       (the id list)
//   #DistroID →  the closed field type a VM source's `distro:` is validated against
//
// A distro's traits are DATA. A Go switch on a distro name is the same duplication in a
// less checkable form: it cannot be validated, cannot be enumerated, and silently defaults
// for any id it does not name — which is exactly how a Debian-family guest came to be
// rendered with Arch package names and an sshd unit it does not have.
//
// Adding a distro is ONE entry here. Nothing else needs editing, and nothing may infer a
// trait from anything other than this table: not from an account name, not from an image
// URL, not from a source kind.

// @go(-): vocabulary-only. The traits reach Go as the four generated TABLES
// (DistroFormats / DistroSSHUnits / DistroInits / DistroIDs), which is all any
// consumer reads — projecting a `DistroTrait` struct and a `Distros` map as well
// would add exported contract-module API with zero callers.
#DistroTrait: {
	// format is the native package format. The four charly implements.
	format: "rpm" | "deb" | "pac" | "apk"
	// ssh_unit is the service name OpenSSH installs. Debian-family ships `ssh`;
	// everyone else ships `sshd`. Getting this wrong leaves a guest unreachable.
	ssh_unit: "ssh" | "sshd"
	// init is the guest's service manager. It selects HOW a service is enabled and
	// started, never a distro-specific command inline in Go.
	init: "systemd" | "openrc"
	// ovmf_family is which vendor's OVMF firmware layout a HOST of this distro ships,
	// selecting where the edk2 images live. It is a property of the distro like the
	// other three, and it lived in a hand-maintained `ovmf_distro_aliases:` map kept in
	// TWO byte-identical copies (charly's charly.yml and plugin-vm's
	// build_defaults.yml) — a table that had to be edited twice for every new distro.
	//
	// OPTIONAL on purpose: a distro with no known family keeps today's behaviour
	// exactly, where ovmfCandidatesForDistro tries the union of all families and the
	// not-found error emits the generic install hint. archarm (aarch64 firmware paths
	// differ) and alpine are unset for that reason, as they were before.
	ovmf_family?: "fedora" | "arch" | "debian"
} @go(-)

#Distros: [string]: #DistroTrait
#Distros: { // @go(-) — see #DistroTrait
	fedora: {format: "rpm", ssh_unit: "sshd", init: "systemd", ovmf_family: "fedora"}
	rhel: {format: "rpm", ssh_unit: "sshd", init: "systemd", ovmf_family: "fedora"}
	centos: {format: "rpm", ssh_unit: "sshd", init: "systemd", ovmf_family: "fedora"}
	rocky: {format: "rpm", ssh_unit: "sshd", init: "systemd", ovmf_family: "fedora"}
	almalinux: {format: "rpm", ssh_unit: "sshd", init: "systemd", ovmf_family: "fedora"}

	// Debian-family: the OpenSSH service is `ssh`, and on releases where it is
	// socket-activated the socket carries the same stem.
	debian: {format: "deb", ssh_unit: "ssh", init: "systemd", ovmf_family: "debian"}
	ubuntu: {format: "deb", ssh_unit: "ssh", init: "systemd", ovmf_family: "debian"}

	arch: {format: "pac", ssh_unit: "sshd", init: "systemd", ovmf_family: "arch"}
	archarm: {format: "pac", ssh_unit: "sshd", init: "systemd"}
	manjaro: {format: "pac", ssh_unit: "sshd", init: "systemd", ovmf_family: "arch"}
	endeavouros: {format: "pac", ssh_unit: "sshd", init: "systemd", ovmf_family: "arch"}
	cachyos: {format: "pac", ssh_unit: "sshd", init: "systemd", ovmf_family: "arch"}

	// Omarchy (omacom/omarchy) is vanilla Arch + Hyprland: Arch's pacman, Arch's
	// openssh (`sshd`), Arch's systemd, Arch's edk2 layout. A DISTINCT id rather than
	// an alias of arch, because #DistroID keys three things Arch does not share: the
	// unattended-install answer format (archinstall JSON, which Arch's own ISO does
	// not ship pre-seeded), the extra [omarchy] pacman repo plus Omarchy's pinned
	// Arch mirror snapshot, and the bootloader (limine + a UKI, not grub). Reusing
	// `arch` would turn every one of those into a branch on a string.
	// x86_64 only — pkgs.omarchy.org publishes no aarch64 tree.
	omarchy: {format: "pac", ssh_unit: "sshd", init: "systemd", ovmf_family: "arch"}

	// Alpine runs OpenRC, not systemd. This row is what makes that a first-class,
	// enumerable property instead of a branch on the string "alpine".
	alpine: {format: "apk", ssh_unit: "sshd", init: "openrc"}
} @go(-)

// #DistroID is the closed id vocabulary, DERIVED from #Distros' own keys so the two can
// never disagree. A VM source's `distro:` is validated against this.
#DistroID: or([for id, _ in #Distros {id}]) @go(-)
