package schema

// os.cue — the `os` KIND's definition. #Os validates ONE value of the `os:` map,
// an S4 key-tagged node declared at DESIGN.cue's #KindAlt beside `source:`, `init:`
// and `builder:`.
//
// WHY THIS FILE EXISTS (SDD, D-LAW-4 / D-SCH-1): `os:` has been a DECLARED kind since
// DESIGN.cue:284 with its shape written at DESIGN.cue:811 — and nothing implemented it.
// That is why `#Source` could not be authored at all: `#SourceFields` requires
// `os!: #Ref @ref(os)`, a reference to a node of a kind no schema admitted. This file
// is the kind, so the reference has a target.
//
// The shape is MIRRORED VERBATIM from DESIGN.cue:811-839, which is the authored
// contract. Nothing here is designed; the one addition is the Go lowering tags, which
// DESIGN has no opinion about.
//
// MIRROR, NOT MOVE: DESIGN.cue stays the authoring schema (it validates an authored
// `charly.yml` with `cue vet -d '#Document'`). This file is the same defs as the
// RUNTIME contract, so a plugin can generate its types and validate a value it is
// handed. Reconciling the two files is opencharly/opencharly#1200's business, not this
// file's — the divergence that matters here is that `os:` had no implementation at all.
//
// A distro's traits are DATA, not a Go switch: design/distro_vocab.cue already states
// that rule for #DistroTrait's {format, ssh_unit, init, ovmf_family}. #Os is that same
// data plus the conversion vocabulary D-SRC-2 requires (package_manager, boot,
// bootstrapper, disk_layout, installer). The two are one concept and #DistroTrait is
// the legacy one — opencharly/opencharly#1200 carries the lift.

// ── the package model #Os and #Source share ──────────────────────────────────

// _packageManager — the package managers charly drives. S3 vocabulary table.
_packageManager: {dnf: {}, apt: {}, pacman: {}, apk: {}, zypper: {}} @vocab(spec)
_packageManagerWord: [for w, _ in _packageManager {w}]

// #OsPackage is a plain package name; #ManagerLocator names the manager explicitly for
// a package the os's own manager does not carry. Two arms, because the second form is
// the escape hatch and must be visibly different from the first.
#OsPackage: =~"^[^: ]+$"
#ManagerLocator: =~"^(dnf|apt|pacman|apk|zypper):[^ ]+$"
#Package: #OsPackage | #ManagerLocator

// ── the download model #Source's remote origins share (D-SRC-1) ──────────────

#Url: =~"^[a-z][a-z0-9+.-]*://[^ ]+$"
#Sha256: =~"^[0-9a-f]{64}$"
#Download: {url!: #Url, sha256!: #Sha256}

// ── the os's own sub-shapes ──────────────────────────────────────────────────

// _initRole: the two roles a service unit plays. S3 vocabulary table.
_initRole: {container: {}, system: {}} @vocab(spec)
_initRoleWord: [for w, _ in _initRole {w}]

// #OsInit names the init node per role, closed over _initRole so a typo in a role key
// is a schema error rather than a silently-ignored service.
#OsInit: {for r, _ in _initRole {(r)!: #Ref @ref(init)}}

// _firmware: how the disk form's boot content starts. S3 vocabulary table.
_firmware: {uefi: {}, bios: {}, uefi_secure: {}} @vocab(spec)
_firmwareWord: [for w, _ in _firmware {w}]

// #Boot — the boot content of the disk form: the firmware it boots under, and the
// kernel a microVM hypervisor boots beside a root drive (DESIGN §14.2).
#Boot: {firmware: *"uefi" | or(_firmwareWord), kernel?: #AbsPath, initrd?: #AbsPath, command_line?: #Text}

// #OsBootstrap — the command that fills a root filesystem of this os.
#OsBootstrap: {install?: #Text, package?: [...#Package], cache_mount?: [...#AbsPath]}

// #OsFormat — the rules a rendered unit must pass, and the local packages it needs.
#OsFormat: {validate?: [...#Text], local_package?: [...#Package]}

// #OsBootstrapper — the from-scratch tool, by the family that ships it.
#OsBootstrapper: {pacstrap?: #Text, debootstrap?: #Text, alpine?: #Text}

// #OsDiskLayout — the partitions an installer of this os creates.
#OsDiskLayout: {efi?: #AbsPath, root?: #AbsPath, swap?: #AbsPath}

// #OsInstaller — the answer file an unattended install reads, and the volume it is on.
#OsInstaller: {answer?: #AbsPath, volume?: #Name}

// ── the node ─────────────────────────────────────────────────────────────────

#Os: {
	description!:     #Text
	family!:          #Name
	version?:         #Text
	package_manager!: or(_packageManagerWord)
	init!:            #OsInit // D-CANDY-3: the os owns the init
	boot?:            #Boot

	// §16.4: the os of the stack's source owns every CONVERSION — the build vocabulary
	// below is that ownership made checkable.
	inherit?:         [...#Name] // the os words whose vocabulary this one extends
	inherit_package?: [...#Package] // the packages every build of this os gets
	bootstrap?:       #OsBootstrap // how a root filesystem of this os is made
	workaround?:      #Text // a known defect of this os, and what the build does about it
	format?:          {[or(_packageManagerWord)]: #OsFormat} // the per-manager format render

	base_user?:    #Name // the account an image of this os adopts
	bootstrapper?: #OsBootstrapper // the from-scratch tools this os can be bootstrapped with
	bootloader?:   #Name // the bootloader an installer of this os writes
	disk_layout?:  #OsDiskLayout // the partitions an installer of this os creates
	installer?:    #OsInstaller // what an unattended install of this os reads
}
