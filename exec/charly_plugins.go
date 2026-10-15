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
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencharly/spec/shellquote"
	"github.com/opencharly/spec/spec"
)

// PluginsEnvVar is the variable the loader reads first (charly/plugin_loader.go: `CHARLY_PLUGIN_DIR`
// PREPENDS a directory ahead of the FHS packaged dir). Setting it on the venue is what turns a
// delivered tree into a served one.
const PluginsEnvVar = "CHARLY_PLUGIN_DIR"

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
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		return nil
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
func EnsurePluginsInDeployVenue(ctx context.Context, ve spec.DeployExecutor, pluginDir, hostVer string) (string, error) {
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
	return remote, nil
}
