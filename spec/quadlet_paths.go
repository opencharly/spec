package spec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencharly/spec/storeroot"
)

// quadlet_paths.go — the on-disk quadlet/systemd PATH helpers + host existence probes
// (QuadletDir / SystemdUserDir / QuadletExists[Instance]), RELOCATED from sdk/kit (#55 coneD
// import-purity: charly core inlines spec.QuadletDir, dropping its sdk/kit import). These are
// pure host-fs probes — os.UserHomeDir + os.Stat + filepath.Join, NO subprocess / NO podman —
// so they live in the universal spec/spec slice (std-lib only, no heavy dep). sdk/kit re-exports
// them (kit/quadlet_paths.go) so existing kit.QuadletDir / kit.QuadletExists plugin call sites
// are untouched; new charly-core consumers reference spec.* directly.

// QuadletDir returns the user-level quadlet directory under the XDG config base.
func QuadletDir() (string, error) {
	base, err := storeroot.Base(storeroot.RootConfig)
	if err != nil {
		return "", fmt.Errorf("determining config base: %w", err)
	}
	return filepath.Join(base, "containers", "systemd"), nil
}

// SystemdUserDir returns the user-level systemd unit directory under the XDG config base.
func SystemdUserDir() (string, error) {
	base, err := storeroot.Base(storeroot.RootConfig)
	if err != nil {
		return "", fmt.Errorf("determining config base: %w", err)
	}
	return filepath.Join(base, "systemd", "user"), nil
}

// QuadletExists checks whether a .container file exists for the given image.
func QuadletExists(boxName string) (bool, error) {
	return QuadletExistsInstance(boxName, "")
}

// QuadletExistsInstance checks whether a .container file exists for the given image/instance.
func QuadletExistsInstance(boxName, instance string) (bool, error) {
	qdir, err := QuadletDir()
	if err != nil {
		return false, err
	}
	qpath := filepath.Join(qdir, QuadletFilenameInstance(boxName, instance))
	_, err = os.Stat(qpath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
