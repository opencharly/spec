// Plugin delivery: the half of the bootstrap that charly_install.go does not carry.
//
// opencharly/charly#916 is the measured gap this file closes. A deploy hands its target a charly
// but NONE of the project's plugins, so a plan step that needs a plugin fails there with the
// plugin's own command missing — the command's output is charly's USAGE text at exit=2, the
// unknown-command code, not a runtime error. The loader reports why, naming both candidates:
//
//	plugin env: unset                      # CHARLY_PLUGIN_DIR
//	                                       # no /usr/lib/charly/plugins
//	                                       # no ~/.cache/charly/plugins
//
// The binary is the body; the plugins are the hands. ensureCharly (charly_install.go) delivers the
// body and this file delivers the hands, over the SAME executor abstraction and with the same
// probe → replicate → present → deliver shape, so a venue ends up with a working charly rather
// than half of one.
//
// The delivered tree is the BAKED layout the loader already understands: provider binaries beside
// `<name>.providers` manifests, pointed at by CHARLY_PLUGIN_DIR — the route
// charly/plugin_baked_classes_test.go calls "the documented way to point charly at a plugin built
// from a working tree" (and the route opencharly/charly#831 fixed for deploy-class words).

package exec

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opencharly/spec/shellquote"
	"github.com/opencharly/spec/spec"
)

// PluginsEnvVar is the variable the loader reads first (charly/plugin_loader.go: `CHARLY_PLUGIN_DIR`
// PREPENDS a directory ahead of the FHS packaged dir). Setting it on the venue is what turns a
// delivered tree into a served one.
const PluginsEnvVar = "CHARLY_PLUGIN_DIR"

// StablePluginsPath is the version- and digest-independent name a venue's plugin tree is reachable
// at. The sdk's plan compiler (deploykit.CompileShellHookStep) emits it as CHARLY_PLUGIN_DIR on
// every venue shell, and this file publishes it here as a symlink to whatever content-addressed
// replica the delivery landed.
//
// The two halves need a shared NAME rather than a shared value: a plan is compiled host-side BEFORE
// any delivery, so it cannot know a digest; the delivery cannot know that a plan will be applied to
// this venue. The stable name is where those two facts meet.
//
// NOT "the one home" - and the earlier comment here claimed it was, which was false. The sdk's plan
// compiler carries its OWN `const StablePluginsPath` for the same pin cycle, so for exactly one pin
// cycle this literal is declared twice: once host-side in the sdk and once here, because the two
// modules have no import edge between them and the name must exist on both sides of the wire. That
// is a deliberate, bounded duplication rather than a second implementation of one behaviour:
// neither declaration derives the other, and neither can be deleted while the other stands.
// Removing it is tracked as opencharly/sdk#399 (sweep the local literal when the pin advances to a
// sdk that exports the constant). Until then, a change to this value must move BOTH declarations in
// the same cutover, or the plan and the delivery stop meeting at one path - and the failure mode is
// silent: the tree is delivered, published at a name no plan looks for, and nothing is served.
const StablePluginsPath = "/tmp/charly-plugins"

// stablePluginsPath resolves the injected override, falling back to the package-wide default.
// A caller that passes a path of its own never touches the shared, non-disposable one.
// stableNameLink is the ONE spelling of the command that points the stable name at the replica.
// `-n` replaces the link atomically rather than moving INTO an existing target, and the command is
// idempotent, so a caller may run it on every path - including a warm replica that needed no transport.
func stableNameLink(remote, stablePath string) string {
	return fmt.Sprintf("ln -sfn %s %s",
		shellquote.ShellQuote(remote), shellquote.ShellQuote(stablePluginsPath(stablePath)))
}

func stablePluginsPath(injected string) string {
	if injected != "" {
		return injected
	}
	return StablePluginsPath
}

// ManifestSuffix is the per-provider manifest the loader reads to learn which words a binary
// answers. A directory without at least one of these is not a plugin tree, and delivering it would
// be the same as delivering nothing while looking like progress.
const ManifestSuffix = ".providers"

// replicatedPluginsPath mirrors replicatedCharlyPath: the same content-addressed /tmp discipline,
// keyed by the tree's digest so an unchanged tree is reused and a changed one is re-delivered.
func replicatedPluginsPath(pluginDir, hostVer string) (string, error) {
	digest, err := dirDigest(pluginDir)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/tmp/charly-plugins-%s-%x", hostVer, digest[:8]), nil
}

// runOK runs a script on the venue and fails unless it exited zero. The executor's ONE capture
// method returns the exit code rather than an error for a non-zero exit, so a caller that ignores
// it cannot tell a successful mkdir from a refused one.
func runOK(ctx context.Context, ve spec.DeployExecutor, script string) error {
	_, stderr, exit, err := ve.RunCapture(ctx, script)
	if err != nil {
		return err
	}
	if exit != 0 {
		return fmt.Errorf("script exited %d (%s): %s", exit, script, strings.TrimSpace(stderr))
	}
	return nil
}

// dirDigest hashes the tree's entries and contents in sorted order, so the value is stable across
// hosts and filesystems and changes exactly when the served bytes change.
func dirDigest(dir string) ([]byte, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s|%d|", rel, info.Mode())
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		// Closed explicitly rather than deferred: a defer here would not run until the whole
		// Walk returned, leaving one open file per tree entry.
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	})
	if err != nil {
		return nil, fmt.Errorf("hashing plugin tree %s: %w", dir, err)
	}
	return h.Sum(nil), nil
}

// CountManifests reports how many `<name>.providers` manifests a directory tree carries. It is the
// honest measure of "is this a plugin tree": a tree with none is empty of capability whatever it
// contains, and every caller here treats zero as a failure rather than a state to proceed from.
func CountManifests(dir string) (int, error) {
	n := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ManifestSuffix {
			n++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("counting plugin manifests in %s: %w", dir, err)
	}
	return n, nil
}

// ValidatePluginTree is the pure refusal behind the delivery: a directory carrying no
// `<name>.providers` manifest is not a plugin tree, and delivering it would leave the venue exactly
// as broken as before while the deploy reported success — #916's own failure mode reproduced as a
// "fix". It is a pure predicate so it can be asserted WITHOUT an executor, which is the only way
// the control can fail for its own reason rather than for a guard that fires first.
func ValidatePluginTree(dir string) error {
	n, err := CountManifests(dir)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("plugin tree %s carries no %s manifest, so it is not a plugin tree", dir, ManifestSuffix)
	}
	return nil
}

// EnsurePluginsInDeployVenue delivers a baked plugin tree into a live spec.DeployExecutor and
// returns the remote directory the venue should export as PluginsEnvVar. It mirrors
// EnsureCharlyInDeployVenue exactly: probe what the venue has, compose the replica path, skip when
// it is already served, otherwise deliver.
//
// A source tree with NO manifests is refused rather than delivered: shipping an empty tree would
// reproduce #916's symptom while looking like a fix, which is the failure mode this whole file
// exists to remove.
// EnsurePluginsInDeployVenue delivers the plugin tree to the venue and publishes it under the
// package-wide StablePluginsPath. Callers that must NOT touch that shared, non-disposable path -
// a host-side test on a developer machine, for instance - use EnsurePluginsInDeployVenueAt with a
// path of their own instead, and then never touch the shared one at all.
func EnsurePluginsInDeployVenue(ctx context.Context, ve spec.DeployExecutor, pluginDir, hostVer string) (string, error) {
	return EnsurePluginsInDeployVenueAt(ctx, ve, pluginDir, hostVer, "")
}

func EnsurePluginsInDeployVenueAt(ctx context.Context, ve spec.DeployExecutor, pluginDir, hostVer, stablePath string) (string, error) {
	if ve == nil {
		return "", fmt.Errorf("ensure plugins in deploy venue: nil executor")
	}
	if err := ValidatePluginTree(pluginDir); err != nil {
		return "", err
	}

	remote, err := replicatedPluginsPath(pluginDir, hostVer)
	if err != nil {
		return "", err
	}

	// probe: the venue already carries a non-empty tree at the replica path.
	_, _, exit, err := ve.RunCapture(ctx, fmt.Sprintf("test -n \"$(ls -A %s 2>/dev/null)\"", shellquote.ShellQuote(remote)))
	if err == nil && exit == 0 {
		// A warm replica still republishes: the probe governs whether the tree must be TRANSPORTED,
		// never whether the name must be REPUBLISHED.
		if perr := runOK(ctx, ve, stableNameLink(remote, stablePath)); perr != nil {
			return "", fmt.Errorf("publishing %s -> %s in deploy venue: %w", stablePluginsPath(stablePath), remote, perr)
		}
		return remote, nil
	}

	// deliver: stream every file into the tree, executables keeping their mode.
	if err := runOK(ctx, ve, "mkdir -p "+shellquote.ShellQuote(remote)); err != nil {
		return "", fmt.Errorf("creating plugin dir %s in deploy venue: %w", remote, err)
	}
	err = filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(remote, rel)
		if dir := filepath.Dir(dst); dir != remote {
			if err := runOK(ctx, ve, "mkdir -p "+shellquote.ShellQuote(dir)); err != nil {
				return err
			}
		}
		return ve.PutFile(ctx, path, dst, uint32(info.Mode().Perm()), false, spec.EmitOpts{})
	})
	if err != nil {
		return "", fmt.Errorf("copying plugin tree into deploy venue %s: %w", remote, err)
	}

	// verify at the boundary: a delivery that did not land is not a delivery.
	n, err := CountManifests(pluginDir)
	if err != nil {
		return "", err
	}
	_, _, exit, err = ve.RunCapture(ctx, fmt.Sprintf("test \"$(find %s -name '*%s' | wc -l)\" -eq %d", shellquote.ShellQuote(remote), ManifestSuffix, n))
	if err != nil || exit != 0 {
		return "", fmt.Errorf("plugin tree at %s does not carry the %d manifest(s) delivered from %s", remote, n, pluginDir)
	}

	// Publish the stable name the PLAN emitted, so the two halves meet at one path. `-n` is
	// deliberate: the link is replaced atomically rather than moved INTO the target when the target
	// already exists, which is the one way this line can silently publish the wrong tree.
	if err := runOK(ctx, ve, stableNameLink(remote, stablePath)); err != nil {
		return "", fmt.Errorf("publishing %s -> %s in deploy venue: %w", stablePluginsPath(stablePath), remote, err)
	}
	return remote, nil
}

// EnsurePluginsInGuest is the vm-deploy sibling of EnsurePluginsInDeployVenue: at PrepareVenue time
// the reverse channel has NOT yet served a guest executor, so the delivery runs over the
// HOST-SURFACE ssh/scp helpers charly_install.go already uses (sshCapture / sshOK / scpInto), with
// the same strategy vocabulary layered on top — auto/scp deliver, skip verifies presence only.
//
// The tree travels as a tar stream because scpInto moves a FILE and a plugin tree is a directory;
// the guest extracts it in place, so the delivered layout is byte-identical to the source's, modes
// included (a provider binary that arrives non-executable is a plugin the loader cannot serve).
// EnsurePluginsInGuest is the guest-side twin of EnsurePluginsInDeployVenue: it publishes under the
// package-wide StablePluginsPath unless the caller injects a path of its own, which is how a leg that
// must not touch that shared, non-disposable location stays away from it entirely.
func EnsurePluginsInGuest(ctx context.Context, ssh SSHArgs, pluginDir, hostVer, strategyRaw string) (string, error) {
	return EnsurePluginsInGuestAt(ctx, ssh, pluginDir, hostVer, strategyRaw, "")
}

func EnsurePluginsInGuestAt(ctx context.Context, ssh SSHArgs, pluginDir, hostVer, strategyRaw, stablePath string) (string, error) {
	switch ResolveCharlyInstallStrategy(strategyRaw) {
	case CharlyInstallAuto, CharlyInstallScp:
		if err := ValidatePluginTree(pluginDir); err != nil {
			return "", fmt.Errorf("ensure plugins in guest: %w", err)
		}
		remote, err := replicatedPluginsPath(pluginDir, hostVer)
		if err != nil {
			return "", err
		}

		// probe: the guest already carries a non-empty tree at the replica path.
		if sshOK(ctx, ssh, fmt.Sprintf("test -n \"$(ls -A %s 2>/dev/null)\"", shellquote.ShellQuote(remote))) {
			if !sshOK(ctx, ssh, stableNameLink(remote, stablePath)) {
				return "", fmt.Errorf("publishing %s -> %s in guest", stablePluginsPath(stablePath), remote)
			}
			return fmt.Sprintf("guest already carries the plugin tree at %s (no re-delivery)", remote), nil
		}

		if !sshOK(ctx, ssh, "mkdir -p "+shellquote.ShellQuote(remote)) {
			return "", fmt.Errorf("creating plugin dir %s in guest", remote)
		}

		archive, err := tarPluginTree(pluginDir)
		if err != nil {
			return "", err
		}
		defer func() { _ = os.Remove(archive) }()

		if err := scpInto(ctx, ssh, archive, remote+".tgz"); err != nil {
			return "", fmt.Errorf("copying plugin tree into guest %s: %w", remote, err)
		}
		if !sshOK(ctx, ssh, fmt.Sprintf("tar -xzf %s -C %s && rm -f %s",
			shellquote.ShellQuote(remote+".tgz"), shellquote.ShellQuote(remote), shellquote.ShellQuote(remote+".tgz"))) {
			return "", fmt.Errorf("extracting plugin tree into %s in guest", remote)
		}

		// Verify at the boundary: the same manifest count must be present in the guest.
		want, err := CountManifests(pluginDir)
		if err != nil {
			return "", err
		}
		if !sshOK(ctx, ssh, fmt.Sprintf("test \"$(find %s -name '*%s' | wc -l)\" -eq %d",
			shellquote.ShellQuote(remote), ManifestSuffix, want)) {
			return "", fmt.Errorf("guest plugin tree at %s does not carry the %d delivered manifest(s)", remote, want)
		}

		// Publish the stable name the PLAN emitted. `-n` replaces the link rather than moving into an
		// existing target, so re-running against a changed tree repoints it instead of nesting a link
		// inside the old directory.
		if !sshOK(ctx, ssh, stableNameLink(remote, stablePath)) {
			return "", fmt.Errorf("publishing %s -> %s in guest", stablePluginsPath(stablePath), remote)
		}
		return fmt.Sprintf("host plugin tree provided at %s, published at %s for deploy use (%s)", remote, stablePluginsPath(stablePath), PluginsEnvVar), nil
	case CharlyInstallSkip:
		if !sshOK(ctx, ssh, "test -n \"$(ls -A /usr/lib/charly/plugins 2>/dev/null)\"") {
			return "", fmt.Errorf("no plugin tree present in guest (charly_install.strategy: skip)")
		}
		return "verified a plugin tree present in guest (strategy=skip)", nil
	default:
		return "", fmt.Errorf("unknown charly_install.strategy: %q", strategyRaw)
	}
}

// tarPluginTree writes the tree to a temporary archive with modes preserved. The guest /tmp tmpfs
// is small on the VMs this runs against, which is why the tree is streamed rather than expanded
// into a second copy on the host — the same reasoning as the image transfer path.
func tarPluginTree(dir string) (string, error) {
	f, err := os.CreateTemp("", "charly-plugins-*.tgz")
	if err != nil {
		return "", fmt.Errorf("creating a plugin archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	// A deterministic writer keeps the archive free of host-specific ordering, so an unchanged tree
	// produces an unchanged archive.
	var names []string
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			names = append(names, path)
		}
		return nil
	}); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	sort.Strings(names)

	zw := gzip.NewWriter(f)
	tw := tar.NewWriter(zw)
	for _, path := range names {
		info, err := os.Lstat(path)
		if err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		in, err := os.Open(path)
		if err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		if _, err := io.Copy(tw, in); err != nil {
			_ = in.Close()
			_ = os.Remove(f.Name())
			return "", err
		}
		_ = in.Close()
	}
	if err := tw.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := zw.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
