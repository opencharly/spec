---
name: vms-catalog
description: |-
  Authoring reference for kind:vm entities in vm.yml — the VmSpec schema, source.kind discriminator (cloud_image vs bootc), base_user adopt pattern, and step-by-step recipes for both source kinds. Use before authoring or editing vm.yml entries.
---

# vms

`vm.yml` is the authoring surface for `kind: vm` entities — VM primitives sourced five ways: a remote cloud-image URL (`source.kind: cloud_image`), an in-repo bootc container image (`source.kind: bootc`), a rootfs built from packages by the distro's own bootstrapper (`source.kind: bootstrap`), the distro's own installer ISO driven unattended by a charly-rendered answers volume (`source.kind: iso`), or ANOTHER VM's snapshot (`source.kind: clone` — the VM-on-VM layering path). Loaded through `charly.yml`'s `import:` (or inline under its root). Entries are resolved by `LoadUnified` into `VmSpec` Go types (generated in `spec/spec/`) and consumed by `charly vm build`, `charly vm create`, and `charly deploy add vm:<name>`.

The VM surface parallels the `candy:` image surface: one YAML entry per entity, kind-keyed, discovered through includes. The Go types that back it live in `/charly-internals:vm-spec`; the rendering paths in `/charly-internals:libvirt-renderer` and `/charly-internals:cloud-init-renderer`.

## File layout

```yaml
# vm.yml — name-first nodes (a per-kind split file loaded via charly.yml `import:`, or inline under its root)
<name>:
  vm:
    source:
      kind: cloud_image | bootc | clone
      # cloud_image branch:
      url: https://…
      checksum: { type: sha256, value?: <hex> }
      base_user: arch                     # adopt this account (see Adopt pattern below)
      distro: debian                      # **REQUIRED on a cloud_image source.** Once this cutover lands, the vm kind's OpValidate rejects a
                                          # source that omits it.** A closed `#DistroID`; `spec/schema/distro_vocab.cue`
                                          # is the single source for the id space and each id's package format, sshd
                                          # unit and init system. Nothing is inferred from `base_user`, an image URL, or
                                          # the source kind — the earlier base_user inference is DELETED, so an unnamed
                                          # distro is an author-time error, not a silent wrong default. The 13 ids cover
                                          # the pacman family (arch/archarm/manjaro/endeavouros/cachyos), the RPM family
                                          # (fedora/rhel/centos/rocky/almalinux), Debian-family (debian/ubuntu) and
                                          # alpine, which is first-class: `distro: alpine` selects apk and OpenRC. Only
                                          # openSUSE is still absent, with no near relative (zypper) — one entry adds it.
                                          # Note also that #DistroID (14 ids, omarchy included) and the embedded `distro:` BUILD vocabulary are
                                          # different sets, and the gap is SILENT: `buildVmSyntheticBox` resolves this field against
                                          # the build vocabulary and on a miss leaves `img.Pkg` unset, so candy installation compiles
                                          # ZERO package steps while `deploy add` reports success. A schema-valid id is therefore not
                                          # automatically a resolvable one.
                                          # Use a bare id: `ResolveDistro` strips at `:` but the cloud-init
                                          # dispatches compare exactly, so `debian:13` yields `openssh`. (That sentence is about the
                                          # BUILD vocabulary — `ResolveDistro` in `sdk/buildkit/config_resolve.go` — not `#DistroID`.)
      cache: ~/.cache/charly/vm-images/       # optional override
      # bootc branch:
      box: <candy: image entry name>           # `box:` source field → a `candy:` image carrying base:/from:
      transport: registry | containers-storage | oci | oci-archive
      rootfs: ext4 | xfs | btrfs
      root_size: 10G                      # optional cap; rest of disk stays unpartitioned
      kernel_args: "…"
      # clone branch (VM-on-VM layering):
      from_vm: <base-entity>              # the base kind:vm entity
      from_snapshot: <name>               # the EXACT snapshot of the base to use (registry name)
      cloud_init_clean: true              # regenerate machine-id + SSH host keys on first boot
    # Hardware (both branches):
    disk_size: 20G | "10 GiB" | 1T         # VIRTUAL size; qcow2 is lazily/sparsely allocated (grows on demand)
    ram: 4G | "8192M"
    cpu: 4                                 # yaml key is singular `cpu` (VmSpec.Cpus)
    machine: q35 | virt | i440fx | pc      # default: host-native
    firmware: bios | uefi-insecure | uefi-secure  # default: bios
    backend: auto | libvirt | qemu         # default: auto
    autostart: true                        # start at host boot (libvirt only — see below)
    # Network:
    network:
      mode: user | bridge | nat | network
      bridge: br0                          # only when mode=bridge
      mac: "52:54:…"                       # optional pin; default stable-from-name
      port_forwards: ["8080:80", "auto:6443"] # "<host>:<guest>", additive to SSH forward; host may be `auto` → free host port auto-allocated at vm-create + persisted in vm_state (the sibling of ssh.port_auto; use it so concurrent beds sharing one entity never collide)
    # SSH + key injection:
    ssh:
      user: arch | root | …                # defaults: cloud_image→"charly", bootc→"root"
      port: 2222                           # host port → guest :22
      key_source: auto | generate | none | /abs/path.pub
      key_injection:
        smbios: auto | enabled | disabled
        cloud_init: auto | enabled | disabled
    # Cloud-init (structured intent):
    cloud_init:
      timezone: UTC
      package: [sudo, spice-vdagent, …]
      runcmd: ["…", …]
      charly_install:
        strategy: auto | scp | skip
    # Libvirt XML knobs (see /charly-internals:libvirt-renderer):
    libvirt:
      devices:
        channels: [{type: unix, name: org.qemu.guest_agent.0}]   # qemu-guest-agent
        graphics: […]
        video: [{model: virtio, vram: 65536, heads: 1, accel3d: false}]
        rng: [{model: virtio, backend: /dev/urandom}]
        memballoon: {model: virtio}
        hostdevs: […]                                            # PCI passthrough (charly vm gpu list)
        filesystems:                                             # virtiofs/9p host↔guest shares
          - {driver: virtiofs, accessmode: passthrough, source: /home/me, target: workspace}
      # memory_backing is auto-paired (memfd + shared) for any virtiofs share
    # Ordered plan steps — INLINE on the entity, same #Step grammar as any other kind:
    plan:
      - check: the guest reached steady state
        id: vm-running
        libvirt: info
        stdout: {contains: running}
    # Named snapshots (the `clone` source arm's `from_snapshot:` target):
    snapshot:
      - name: golden
```

### autostart — start the VM at host boot

`autostart: true` sets libvirt's domain autostart flag. Because charly VMs run under
`qemu:///session` (no portable user-level `virtqemud.socket` to socket-activate at
boot), `charly vm create` also enables `loginctl enable-linger` (idempotent) and
writes + enables a per-VM user oneshot `charly-autostart-<domain>.service` that
`virsh -c qemu:///session start`s the domain at boot (`charly vm destroy` removes it).
**Requires `backend: libvirt`** — validation rejects `autostart: true` with
`backend: qemu`. Reapplied on every `charly vm create` / `charly update`. Pair with a
1 TB+ `disk_size` freely — the qcow2 is sparse, so a large virtual disk costs only
the bytes written.

### filesystems — virtiofs host directory shares

`libvirt.devices.filesystems[]` exposes a host directory inside the guest:
`{driver: virtiofs, accessmode: passthrough, source: <host dir>, target: <tag>}`.
The shared-memory backing virtiofs requires (`memfd`/`shared`) is **auto-paired**
— you don't declare `memory_backing` yourself. The guest mounts the `target:` tag
with the `/charly-distros:workspace-mount` layer (a systemd `.mount` unit for the
`workspace` tag → `/workspace`) or any `mount -t virtiofs <tag> <dir>`.

### graphics `listen:` field — three accepted shapes

`graphics[].listen` is how you control the `<listen>` children of a
`<graphics>` element. Three equivalent shapes, all unmarshal to the same
internal list:

```yaml
# (1) Scalar address (shorthand for one TCP listener):
listen: 127.0.0.1

# (2) Single map (explicit type control — socket | address | network):
listen:
  type: socket              # libvirt auto-allocates the UNIX socket path
# or:
listen:
  type: address
  address: 127.0.0.1

# (3) List of maps (multiple listeners on one <graphics>):
listen:
  - type: socket
  - type: address
    address: 127.0.0.1
```

**Prefer `type: socket` for charly-managed VMs.** virt-manager and
`remote-viewer --connect qemu+ssh://…` auto-forward UNIX sockets over
the libvirt RPC channel — GUI clients work out of the box against a
remote libvirt with zero `ssh -L` setup. TCP loopback listeners are
never auto-tunneled, by design. See `/charly-vm:arch-cloud-vm`
"Connecting from a remote workstation".

## source.kind: cloud_image

Use when the VM is built from an **externally published qcow2** (Arch cloud image from pkgbuild.com, Fedora Cloud, Ubuntu Cloud, Debian Cloud, CentOS Cloud, etc.). The build pipeline fetches the URL, integrity-checks it via sha256 (sidecar auto-resolved when `checksum.value` is empty), creates a qcow2 overlay at `spec.disk_size`, renders a NoCloud seed ISO, and hands off to libvirt/QEMU.

Canonical example: `/charly-vm:arch-cloud-vm` — the most fully documented cloud_image VM, and the one to **read before authoring another one** (it is not the only one: `kind: cloud_image` sources also ship in `charly/charly.yml`, `distro-fedora` and `distro-arch`). It documents the non-obvious decisions learned the hard way:

- **BIOS firmware is usually the right default.** Distribution cloud images ship both a BIOS boot partition and an EFI System Partition, but the ESP's bootloader binary often has an **embedded grub.cfg that predates the maintainer's latest `/etc/default/grub`** (Arch's upstream issue with `fbcon=nodefer`). BIOS boot reads `/boot/grub/grub.cfg` directly from the root fs, which is always current.
- **virtio-gpu, not QXL.** See `/charly-internals:libvirt-renderer` "video model choice" — virtio-gpu is the modern default for Linux guests.
- **Generous resource sizing.** `pacman -S spice-vdagent` pulls in GTK3 + X11 (~200 MB download, ~1 GB installed); running at 2 GiB RAM stalls cloud-init. Size for the workload: 8 GiB / 4 cpus is reasonable for a workstation-class dev VM.

### Authoring a new cloud_image VM

1. Find the upstream qcow2 URL + verify a sha256 sidecar exists (<url>.SHA256 / .sha256 / .sha256sum).
2. Identify the **pre-existing user account** in the upstream image (`arch`, `alpine`, `ubuntu`, `fedora`, `debian`, `cloud-user`, etc.). `base_user:` selects the account to adopt and NOTHING else — it no longer steers any render branch, because the base_user inference is deleted. **`distro:` is required on a `cloud_image` source and the vm kind's `OpValidate` rejects a source that omits it.** Do NOT reason from which dispatch tables an id appears in: the render's dispatch union and the vocabulary `candy_select` resolves against are DIFFERENT SETS, and an id in the first but not the second is exactly where the silent zero-package failure lives.** They are: `composePackages` keys on {`debian`, `ubuntu`} (omitting picks `openssh` over `openssh-server`, so cloud-init hard-fails `E: Unable to locate package openssh`); `sshUnitForDistro` keys on the same two (omitting yields unit `sshd` where Debian needs `ssh`); `pacmanFamily`, i.e. `FormatForDistroID(distro) == "pac"`, keys on **five** ids — {`arch`, `archarm`, `manjaro`, `endeavouros`, `cachyos`} — and omitting drops the guest onto the `packages:` path instead of `pacman -Sy --needed` plus the sshd try-restart guard, so cloud-init's bare `pacman -S` re-triggers exactly the host-key regen race described in `/charly-internals:cloud-init-renderer` (its D15 section); and the init branch keys on {`alpine`} (omitting renders systemd onto an OpenRC guest, and the VM boots unreachable). Because nothing is inferred, a CachyOS, Manjaro, EndeavourOS or archarm image is in the pacman family when — and only when — it SAYS so; all four are first-class ids in the vocabulary, so none of them needs to borrow `arch`. An omitted distro is rejected at author time rather than silently leaving the pacman family and taking the racy install path; likewise an Alpine image whose account is not literally `alpine`. The qualifier is load-bearing in both directions, and both are statements about the MECHANISM, not permissions that compete with the rule above: an Arch derivative shipping an `arch` account happens to resolve to `arch` and render correctly, while a Debian image shipping an `alpine` account resolves to `alpine` and breaks. Neither changes what to write — **the SET rule stated at the top of this section governs** — a Manjaro image writes `distro: arch`, whatever its account happens to make the inference do. Not `manjaro`: that is a `pacmanFamily` key, so cloud-init would render it correctly, but it is **not** one of the five ids `candy_select` resolves against, so candy installation would compile nothing. The two tables are different sets, and this clause is scoped to the candy vocabulary. **Always SET `distro:`, as a BARE id from the closed `#DistroID` vocabulary.** Two consumers read this field against **different sets**, which is the whole difficulty. The cloud-init dispatches resolve it through the generated `spec.DistroFormats` / `DistroSSHUnits` / `DistroInits` tables — 13 ids, no inference from `base_user`, and no fallback for a miss. `buildVmSyntheticBox` (`candy/plugin-fleet/candy_select.go`, not source-kind gated) instead resolves this field — or `base_user` when empty — against a **five-id vocabulary**, `{arch, cachyos, debian, fedora, ubuntu}`, with exact-then-base-name lookup and no alias or `ID_LIKE` rescue; a miss leaves `img.Pkg` unset and candy installation compiles **zero package steps silently** while `deploy add` reports success. **If the guest is one of those five, name it.** **Alpine is the case that must not be generalised: set `distro: alpine`.** It IS a vocabulary member now (`alpine: {format: apk, ssh_unit: sshd, init: openrc}`), and naming it is what selects apk plus the OpenRC path — any other value renders systemd onto a guest that has none and the VM boots unreachable. Alpine's candy installation is unresolvable either way; losing the packages is recoverable, losing the boot is not. **For a guest outside the five, naming a near relative is a workaround, not a repair** — `arch` for manjaro/archarm/endeavouros, `fedora` for Rocky or AlmaLinux, `ubuntu` for a Ubuntu derivative. It makes candy steps compile, but it selects **that** distro's DistroDef, version tags and repositories: fedora's carries `version: "43"`, so candies' `fedora:43` sections apply to a Rocky guest and its rpm templates reach for COPR, which is Fedora infrastructure. The lookup succeeding is not the install working. The repair is a vocabulary entry, which is a product gap; openSUSE has neither an entry nor a near relative, since its manager is zypper. Use a bare id — `ResolveDistro` strips at `:` while the dispatches compare exactly, so `distro: debian:13` yields `openssh` instead of `openssh-server`. (The `rocky: fedora` line in `charly.yml` is `ovmf_distro_aliases`, an OVMF FIRMWARE table, not consulted here.) Conversely a wrong id that the vocabulary DOES resolve is actively harmful — `distro:` also selects the guest's package manager for candy installation via `buildVmSyntheticBox` (`candy/plugin-fleet/candy_select.go`, with no source-kind gate), whose own comment records a candy deploy running `pacman` on a debian guest and failing exit 127, and a missing `DistroDef` compiling zero package steps for every candy, silently. `base_user: debian` or `ubuntu` without it resolves to the empty distro, which picks `openssh` instead of `openssh-server` (cloud-init hard-fails `E: Unable to locate package openssh`) and unit `sshd` where Debian needs `ssh`; an Alpine image whose account is named anything but `alpine` renders the systemd path onto an OpenRC guest. All three boot unreachable. This becomes `source.base_user:` — triggers the adopt pattern described below.
3. Start from `/charly-vm:arch-cloud-vm` as a template. Change `url`, `base_user`, **`distro:`** (the template ships it LIVE as `distro: arch`; it is required on every cloud_image source, so change the value rather than expecting to uncomment it — see the rule above), and the distro-specific cloud_init `package:` and `runcmd:`.
4. Pick firmware: default to `bios` unless the upstream image explicitly requires UEFI (e.g., secure boot lock-in).
5. Run `charly vm build <name>` — observe the fetched qcow2 sha256 + rendered seed ISO path.
6. Run `charly vm create <name>` + `charly vm ssh <name>` to verify cloud-init completed.

## source.kind: bootc

Use when the VM is built from an **in-repo bootc container image** (a `candy:` image entry with `bootc: true`). `charly vm build` runs `bootc install to-disk --via-loopback` inside a privileged container to produce the qcow2/raw disk.

No bootc VM ships in the repo today; a bootc `vm:` node pairs a `candy:` image carrying `bootc: true` with the VM hardware spec. By convention a `-bootc` suffix marks the bootc VM entity (distinguished from an equivalent container-form deploy by the `vm:` kind key, not by the name).

### Authoring a new bootc VM

1. Ensure the paired container image has `bootc: true` declared and builds cleanly.
2. Add a `vm:` node with `source.kind: bootc` + `source.box: <entry-name>` (the bootc source field is `box:`, not `image:`).
3. Size disk/ram/cpus for the workload (see the relevant per-pod or `/charly-distros:<name> / /charly-languages:<name> / /charly-infrastructure:<name> / /charly-tools:<name>` skill's VM Configuration section for the authoritative numbers).
4. Run `charly vm build <vm-name>`. See `/charly-vm:vm` known-caveats section for bootc-specific gotchas (rootful storage split, nested-container `--transport containers-storage`, loopback device mount namespace).

## source.kind: clone — build a VM on top of another VM

Use when the new VM is a LAYER on top of an existing VM: the base is
another `kind: vm` entity, and the exact base state is pinned by a
NAMED SNAPSHOT of it. This is the VM-on-VM layering primitive — the VM
analog of a container image's `FROM <base>` — and it is what makes a
provisioned VM reusable as a base for many children without re-running
its install.

```yaml
base-vm:
    vm:
        source:
            kind: cloud_image
            distro: arch
            url: https://fastly.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2
            base_user: arch
        disk_size: 20G
        # Declarative snapshots: captured by `charly vm snapshot capture-declared`
        # once the base is provisioned (idempotent).
        snapshot:
            - name: golden
              description: provisioned base state for clones

clone-vm:
    vm:
        source:
            kind: clone
            from_vm: base-vm
            from_snapshot: golden
            cloud_init_clean: true
        disk_size: 20G
```

### Authoring a new clone VM

1. Provision the base VM and capture a named snapshot of its provisioned
   state: `charly vm snapshot create-consistent <base> <name>` (guest-
   consistent) or declare the `snapshot:` block on the base entity and
   run `charly vm snapshot capture-declared <base>` (idempotent).
2. Add a `vm:` node with `source.kind: clone` + `source.from_vm:` +
   `source.from_snapshot:` (both required; the snapshot must exist in
   the base's registry at `~/.local/share/charly/vm/charly-<base>/snapshots/`).
3. `cloud_init_clean: true` (the `charly vm clone` default) injects
   `cloud-init clean --machine-id --logs` so machine-id + SSH host keys
   regenerate on first boot — two clones never collide.
4. Run `charly vm build <clone>` — materializes a fresh qcow2 overlay
   whose backing file is the parent snapshot's frozen external disk (the
   base bytes are shared, not copied; the parent snapshot's refcount is
   bumped, and `charly vm snapshot delete` refuses while clones
   reference it). Then `charly vm create <clone>` + `charly vm ssh`.
5. The one-command form: `charly vm clone <new> --from <base>[@<snap>]`
   writes the declaration AND builds the disk.

### Container ↔ VM switching (layered boxes)

A box (a `candy:` image entry) is the single definition for a layered
system. Deploy it as a container OR as a VM by changing the deploy node
only — the candies, env, ports, and check plan are identical:

```yaml
# Same box, two substrates:
my-system:
    pod:
        image: my-box            # container form
# vs:
my-system-vm:
    vm:
        from: my-box-vm          # VM form — entity with source.kind: bootc, box: my-box
```

For a VM-on-VM layered system, the base box's VM is cloned
(`source.kind: clone`) and the child's own candies are applied in-guest
via `charly deploy add vm:<child>` — the same InstallPlan IR that pod
deploys render into a Containerfile.

## Adopt pattern: base_user (cloud_image only)

Mirrors the container-side `base_user:` + `user_policy: adopt` pattern documented in `/charly-image:image` "user_policy". The key insight: **don't recreate accounts cloud-init already shipped** — just append the SSH pubkey and move on.

When `source.base_user:` is set, the cloud-init renderer (`/charly-internals:cloud-init-renderer::composeUsers`) emits a merge-by-name entry:

```yaml
# Rendered user-data
users:
  - default
  - name: <base_user>
    ssh_authorized_keys:
      - ssh-ed25519 AAAA…
```

cloud-init interprets `users: [default, {name: <base_user>}]` as "keep the distro's default account untouched, append SSH key to the named account". Result: no `useradd`, no sudoers rewrite, no shell change, no home-directory relocation — just the pubkey lands in `~<user>/.ssh/authorized_keys` on first boot.

`spec.ssh.user` defaults to `source.base_user`, so `charly vm ssh <name>` connects as the adopted account without extra declaration.

Leave `base_user:` empty **only** when the upstream has no default account — in which case author a full custom user entry in `cloud_init.users:` with sudo/groups/shell fields. Don't do this when adopting works; `useradd`-at-first-boot races with other cloud-init modules and is harder to reason about.

## charly_install.strategy: auto (cloud_image)

`charly_install.strategy: auto` in `cloud_init:` wires `charly`'s in-guest installer (`/charly-internals:cloud-init-renderer` → emitted `runcmd:` entries) so the provisioned VM comes up with `charly` already installed. Lets `charly deploy add vm:<name>` apply host-deploy-style layer recipes inside the VM over SSH without a bootstrap round-trip. See `/charly-internals:cloud-init-renderer` for the emission + handshake.

`strategy: skip` skips the step entirely — useful when the VM will be managed by something other than `charly` after provisioning. (`none` is NOT a value: the closed schema is `auto | scp | skip` and `none` hard-fails it.)

## Validation rules

Load-time errors raised by the closed `#Vm` CUE schema (`spec/schema/vm.cue`, see `/charly-internals:vm-spec`):

- `source.kind` must be one of `cloud_image`, `bootc`, `clone`, `imported`, `bootstrap`; each arm requires its own fields and forbids the others'.
- `cloud_image` requires `url:`; `bootc` requires the `box:` source field resolving to a `candy:` image entry (carrying `base:`/`from:`); `clone` requires `from_vm:`+`from_snapshot:`; `imported` requires `libvirt_name:`+`disk_path:`+`disk_format:`; `bootstrap` requires `builder:`+`distro:`.
- `firmware:` must be one of `bios`, `uefi-insecure`, `uefi-secure`; `uefi-secure` additionally requires an explicit `libvirt.features.smm: true`.
- `network.mode:` must be one of `user`, `bridge`, `nat`, `network`.
- `ssh.key_source:` must parse as `auto`, `generate`, `none`, or an absolute path; `ssh.port` and `ssh.port_auto` are mutually exclusive.
- `ssh.key_injection.smbios` / `.cloud_init` must be one of `auto`, `enabled`, `disabled`.
- the `libvirt:` subtree is modeled + closed by `#LibvirtDomain` in the same schema — unknown keys and bad enums fail fast.

## Migration from legacy (box.bootc / box.vm / box.libvirt)

Projects predating this schema had three coupled fields on image entries (`candy:` nodes carrying `base:`/`from:`): `bootc: true`, `vm: {...}`, `libvirt: [...]`. **Two** were deleted in the hard cutover — `vm:` and `libvirt:`. `bootc: true` is LIVE and stays on the image entry (`spec/schema/box.cue` declares `bootc?: bool`); it marks the image bootable, and a separate `kind: vm` entity with `source.kind: bootc` references it via `source.box:`, exactly as `/charly-image:image` states. Those two fields are gone, and the step that harvested them into VM nodes lived in the migration chain that was **removed at the `2026.186.2323` migration-baseline reset**. So `charly migrate` no longer converts those legacy fields: a config still carrying them predates the supported schema floor (`2026.174.1100`) and is **unmigratable** — `charly migrate` refuses it (`predates the supported floor … re-author against the current schema`) and changes nothing on disk. Re-author such a VM by hand as a name-first `<name>: {vm: {…}}` node (the shape this skill documents).

For a config already within the migratable window (`[floor, HEAD)`), `charly migrate` is one idempotent command that brings it to the current schema. See `/charly-build:migrate` for the floor/HEAD gate and the full command reference, and `/charly-internals:cutover-policy` for why hard-cutover was the chosen policy.

## Cross-References

- `/charly-vm:vm` — the `charly vm build/create/start/stop/ssh/console` command family
- `/charly-build:migrate` — `charly migrate` conversion from legacy
- `/charly-core:deploy` — `charly deploy add vm:<name>` for in-guest layer application
- `/charly-vm:arch-cloud-vm` — canonical cloud_image VM
- `/charly-internals:vm-spec` — Go type reference
- `/charly-internals:libvirt-renderer` — libvirt XML emission
- `/charly-internals:cloud-init-renderer` — NoCloud seed ISO + user-data emission
- `/charly-internals:ovmf` — UEFI firmware path resolution (when `firmware:` ≠ `bios`)
- `/charly-internals:vm-deploy-target` — the external vm deploy in the InstallPlan pipeline
- `/charly-internals:cutover-policy` — Hard Cutover by Default policy
- `/charly-distros:cloud-init` — guest-side cloud-init layer (pairs with host-side `cloud_init:` emission)
- `/charly-distros:qemu-guest-agent` — virtio-serial channel for host↔guest comms

Live-deploy verification: see /charly-check:check (the 11 Testing Standards) and /charly-internals:disposable.
