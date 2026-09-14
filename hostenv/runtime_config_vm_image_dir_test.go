package hostenv

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRuntimeConfig_VmImageDirRoundTrips proves the new vm.image_dir setting is a
// real config field: it loads back from config.yml. The resolver that consumes it
// (sdk/vmshared.VmDiskRoot) and the settings surface that writes it
// (plugin-settings) are covered in their own repos; this pins the field in the
// module that owns the config shape.
func TestRuntimeConfig_VmImageDirRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("vm:\n  image_dir: /srv/vm-images\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := RuntimeConfigPath
	defer func() { RuntimeConfigPath = orig }()
	RuntimeConfigPath = func() (string, error) { return path, nil }

	cfg, err := LoadRuntimeConfig()
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if cfg.Vm.ImageDir != "/srv/vm-images" {
		t.Fatalf("cfg.Vm.ImageDir = %q, want /srv/vm-images", cfg.Vm.ImageDir)
	}
}
