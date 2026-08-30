// CUE schema for the `distro` kind. #Distro validates ONE value of the `distro:`
// map (DistroDef) — the build vocabulary. CLOSED: every authored key is modeled
// (an unknown key is a typo). TEXT/TEMPLATE fields are Go text/template — plain
// `string`, never parsed. #CacheMount / #PhaseSet / #PhaseTemplates are shared
// (_common.cue).

#Distro: {
	inherits?:         string & =~"^[a-z0-9]+(-[a-z0-9]+)*$"
	inherit_packages?: bool @go(InheritPackages)
	version?:          string & =~"^[0-9]+(\\.[0-9]+)*$"
	bootstrap?:        #Bootstrap
	workaround?: [...string] @go(Workarounds)
	format?: {[string]: #Format} @go(Format,type=map[string]*Format)
	base_user?:        #BaseUser @go(BaseUser,optional=nillable)
	pacstrap?:         #Pacstrap @go(Pacstrap,optional=nillable)
	debootstrap?:      #Debootstrap @go(Debootstrap,optional=nillable)
	alpine_bootstrap?: #AlpineBootstrap @go(AlpineBootstrap,optional=nillable)
	bootloader?:       #Bootloader @go(Bootloader,optional=nillable)
	disk_layout?:      #DiskLayout @go(DiskLayout,optional=nillable)
	dnf?:              #Dnf @go(Dnf,optional=nillable)
	installer?:        #DistroInstaller @go(Installer,optional=nillable)
}

// install_cmd is the bootstrap command; ubuntu sets it to "" (kept WITHOUT
// `& !=""` so the empty-string base case validates).
#Bootstrap: {
	install_cmd: string @go(InstallCmd)
	package?: [...string]
	cache_mount?: [...#CacheMount] @go(CacheMount)
}

#Format: {
	cache_mount?: [...#CacheMount] @go(CacheMount)
	section_field?: {[string]: "list" | "list_of_maps"} @go(SectionFields)
	uninstall_template?: string    @go(UninstallTemplate)
	phase?:              #PhaseSet @go(Phases,optional=nillable)
	validate?: [...#FormatRule]
	secondary?: bool
	local_pkg?: #LocalPkg @go(LocalPkg,optional=nillable)
}

#FormatRule: {
	field: string & !=""
	rule:  string & !=""
}

// #LocalPkg — the local_pkg INSTALL machinery only (the source-build fields
// pkg_glob/source_sentinel/build_template/dep_builder were removed with the
// pkg/ source-build cutover: the `charly generate-packages` plugin builds the
// package now, so the deploy-time + image-build paths only INSTALL the published
// package via install_template/download_template).
#LocalPkg: {
	install_template:   string & !="" @go(InstallTemplate)
	probe:              string & !=""
	download_template?: string @go(DownloadTemplate)
}

#Pacstrap: {
	base_package?: [...string] @go(BasePackages)
	extra_repo?: [...#PacstrapRepo] @go(ExtraRepos)
	runtime_pacman_conf?: string @go(RuntimePacmanConf)
}
#PacstrapRepo: {
	name:      string & !=""
	server:    string & =~"^https?://"
	siglevel?: string @go(SigLevel)
}
#Debootstrap: {
	suite?:      string
	mirror?:     string & =~"^https?://"
	variant?:    string
	components?: string
	include_package?: [...string] @go(IncludePackages)
	base_package?: [...string] @go(BasePackages)
	extra_repo?: [...#DebootstrapRepo] @go(ExtraRepos)
}
#DebootstrapRepo: {
	name:        string & !=""
	url:         string & =~"^https?://" @go(URL)
	suite?:      string
	components?: string
}
#AlpineBootstrap: {
	mirror_url?: string & =~"^https?://" @go(MirrorURL)
}
#Bootloader: {
	install_template?:   string @go(InstallTemplate)
	initramfs_template?: string @go(InitramfsTemplate)
	fstab_template?:     string @go(FstabTemplate)
}

// #DiskLayout describes how a bootstrap VM's disk is partitioned and mounted, for the
// distros whose on-disk shape is part of their identity rather than a per-VM choice.
//
// Both fields are optional and both default to what charly did before this def existed,
// so a distro that omits the block builds exactly the disk it built before: a bare root
// filesystem with the ESP at /boot/efi.
#DiskLayout: {
	// esp_mount_point is where the EFI System Partition is mounted, relative to the guest
	// root. Defaults to "/boot/efi".
	//
	// This is NOT cosmetic. Omarchy mounts its ESP at "/boot", which is what
	// limine-entry-tool and ESP_PATH assume; a loader written to the other path leaves an
	// unbootable disk and the build reports success either way.
	esp_mount_point?: string & =~"^/" @go(EspMountPoint)

	// subvolume, when non-empty, makes the root filesystem a btrfs subvolume layout
	// instead of a bare filesystem. Requires the VM source's rootfs to be "btrfs".
	//
	// Exactly one entry must mount at "/" — it becomes the root that the others nest
	// under, so without it there is nothing to mount them into. Enforced by OpValidate
	// rather than by CUE, because "exactly one element of this list has field X = Y" is
	// not expressible in a closed struct.
	subvolume?: [...#Subvolume] @go(Subvolume)
}

// #Subvolume is one btrfs subvolume in a #DiskLayout.
#Subvolume: {
	// name is the subvolume as created, e.g. "@" or "@home".
	name: string & !=""

	// mount_point is the guest-absolute path it is mounted at, e.g. "/" or "/home".
	mount_point: string & =~"^/" @go(MountPoint)

	// mount_options are extra comma-joined mount options, e.g. "compress=zstd,noatime".
	// `subvol=<name>` is always prepended by the emitter and must not be repeated here.
	mount_options?: string @go(MountOptions)
}
#BaseUser: {
	name: string & !=""
	uid:  int & >=0 @go(UID,type=int)
	gid:  int & >=0 @go(GID,type=int)
	home: string & =~"^/"
}
#Dnf: {
	max_parallel_downloads?: int & >=1 @go(MaxParallelDownloads)
	fastestmirror?:          bool
}

// --- resolve-to-envelope wire type (Cutover M, the long pole; SDD conversion,
// per the standing operator directive: a hand-written wire struct not yet
// CUE-sourced is conversion-in-progress, never a sanctioned exception).
// candy/plugin-distro resolves an authored `distro:` build-vocabulary entity
// into a ResolvedDistro the kernel's build engine consumes without importing
// the concrete spec.Distro. Written out explicitly (not embedding #Distro) so
// every field's required/optional state is independently auditable against
// the former hand type. The host keeps RenderTemplate + the cache-mount vocab
// (per the plan); the plugin owns the distro KNOWLEDGE (schema/typed
// shape/validation). PrimaryFormat()/LocalPkgFormat() are pure Go METHODS —
// CUE cannot express them — and stay hand-written in spec/distro_methods.go
// (mirrors Op.Kind() in spec/charly_methods.go: a method, not a type).
#ResolvedDistro: {
	inherits?:         string @go(Inherits)
	inherit_packages?: bool   @go(InheritPackages)
	version?:          string @go(Version)
	bootstrap?:        #Bootstrap @go(Bootstrap)
	workaround?: [...string] @go(Workarounds)
	format?: {[string]: #Format} @go(Format,type=map[string]*Format)
	base_user?:        #BaseUser        @go(BaseUser,optional=nillable)
	pacstrap?:         #Pacstrap        @go(Pacstrap,optional=nillable)
	debootstrap?:      #Debootstrap     @go(Debootstrap,optional=nillable)
	alpine_bootstrap?: #AlpineBootstrap @go(AlpineBootstrap,optional=nillable)
	bootloader?:       #Bootloader      @go(Bootloader,optional=nillable)
	disk_layout?:      #DiskLayout      @go(DiskLayout,optional=nillable)
	dnf?:              #Dnf             @go(Dnf,optional=nillable)
	installer?:        #DistroInstaller @go(Installer,optional=nillable)
	raw?: bytes @go(Raw,type=RawBody)
}

// #DistroConfig is the `distro:` build-vocabulary container — the whole distro
// map keyed by distro name (charly/charly.yml's `distro:` section). #55 step
// 3-III: relocated from sdk/buildkit so every charly/plugin file that uses ONLY
// this value type drops its sdk/buildkit import for spec (the import-purity
// lever). Its vocabulary-resolution methods (ResolveDistro / ResolveInherits /
// AllFormatNames / FindFormat / ValidFormat / ExpandPackageInheritance) are pure
// Go METHODS on the type's own map — CUE cannot express them — and stay
// hand-written in spec/distro_config_methods.go (mirrors ResolvedDistro's
// PrimaryFormat in spec/distro_methods.go: a method, not a type).
#DistroConfig: {
	distro: {[string]: #ResolvedDistro} @go(Distro,type=map[string]*ResolvedDistro)
}

// #DistroResolveInput carries one opaque distro body to project.
#DistroResolveInput: {
	distro!: bytes @go(Distro,type=RawBody)
}

// #DistroResolveReply wraps the resolved distro.
#DistroResolveReply: {
	resolved?: #ResolvedDistro @go(Resolved,optional=nillable)
}

// #DistroInstaller — the UNATTENDED-INSTALL renderer vocabulary for a
// `source.kind: iso` VM. It is to that arm exactly what #Init.service_schema is to
// `service:`: THIS DISTRO owns the answer-file FORMAT, while the vm entity's
// `source.installer:` owns the DATA (#VmInstaller). Nothing may infer an installer
// format from a URL, an ISO name or a base_user — the same rule distro_vocab.cue's
// header states for every other distro trait, and for the same reason.
//
// Five real formats live behind this one shape: archinstall JSON (Omarchy, Arch),
// kickstart (Fedora/RHEL), preseed (Debian), Subiquity autoinstall (Ubuntu) and
// AutoYaST (SUSE). Each is keyed by distro and by nothing else.
#DistroInstaller: {
	// volume_id is the filesystem LABEL the installer looks for. archinstall and
	// cloud-init NoCloud use "cidata"; Anaconda kickstart uses "OEMDRV". Case is
	// preserved verbatim by the ISO writer, so write it exactly as the installer
	// matches it.
	volume_id: string & =~"^[A-Za-z0-9_-]{1,32}$" @go(VolumeID)
	// fs is how the answers volume is packed. iso9660 is correct for every installer
	// verified so far; vfat exists for one that matches its label case-sensitively in
	// a way ISO 9660 cannot represent.
	fs?: *"iso9660" | "vfat" @go(FS)
	file: [...#DistroInstallerFile] @go(Files)
	// boot_arg is appended to the INSTALLER's kernel cmdline when the installer needs
	// to be pointed at its answers explicitly ("inst.ks=hd:LABEL=OEMDRV:/ks.cfg").
	// Empty for an installer that auto-discovers the labelled volume.
	boot_arg?: string @go(BootArg)
	// done is how the build path learns the install finished. poweroff: the installer
	// powers the guest off and the build waits for the process to exit. marker: the
	// installer writes marker_path into the target root, polled over the guest agent.
	// NEVER a sleep.
	done?:        *"poweroff" | "marker" @go(Done)
	marker_path?: string & =~"^/"        @go(MarkerPath)
}

// #DistroInstallerFile is ONE file placed on the answers volume.
#DistroInstallerFile: {
	// path is relative to the volume root.
	path: string & =~"^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$"
	// content is a Go text/template rendered against #InstallerSeedContext.
	content: string
	mode?:   string & =~"^0[0-7]{3,4}$"
	// when is a Go-template guard: the file is emitted only when it renders non-empty
	// and not "false". This is what lets ONE vocabulary express a whole optional
	// matrix — an ssh key file only when keys were given, a "defer provisioning"
	// sentinel INSTEAD of credentials, an encryption marker only when encrypting —
	// with no Go branch anywhere.
	when?: string
}

// #InstallerSeedContext is what a #DistroInstallerFile.content template renders
// against. Host-computed. The PLAINTEXT password never appears here: the caller
// crypt()s it first, so no plaintext reaches a template, a log, or a temp file.
#InstallerSeedContext: {
	hostname?: string @go(Hostname)
	timezone?: string @go(Timezone)
	locale?:   string @go(Locale)
	keyboard?: string @go(Keyboard)

	username?:      string @go(Username)
	full_name?:     string @go(FullName)
	email?:         string @go(Email)
	password_hash?: string @go(PasswordHash)

	disk?:    string @go(Disk)
	encrypt?: bool   @go(Encrypt)
	// encryption_password is the LUKS passphrase, and it is plaintext by necessity:
	// the installer needs it to create the volume. It is only ever set when
	// encrypt is true, and it makes the rendered answers volume a secret.
	encryption_password?: string @go(EncryptionPassword)

	ssh_authorized_key?: [...string] @go(SSHAuthorizedKeys)
	defer_provisioning?: bool        @go(DeferProvisioning)

	// disk_size_bytes is the TARGET DISK's size in bytes, from the vm entity's
	// disk_size. It is here because a real installer answer file states partition
	// sizes as absolute numbers, not as "the rest of the disk": archinstall, for one,
	// indexes partition['size'] with no default and has no fill-remaining sentinel, so
	// a template that cannot see the disk size cannot describe a root partition at all.
	//
	// BYTES, not a suffixed string, because the arithmetic happens in the template and
	// a template is the wrong place to parse "40G".
	disk_size_bytes?: int @go(DiskSizeBytes,type=int64)

	// answer carries the vm entity's per-distro extras verbatim.
	answer?: {[string]: string} @go(Answers)
}
