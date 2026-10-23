package hostenv

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRuntimeConfig points the RuntimeConfigPath seam at a temp file with the
// given body for the duration of the test.
func writeRuntimeConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}
	orig := RuntimeConfigPath
	RuntimeConfigPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { RuntimeConfigPath = orig })
}

// TestResolveRuntime_NamespaceFromSettings pins the settings-file level of the
// containerd namespace: `containerd.namespace` wins over the engine capability's
// default. The engine is forced to containerd (the namespace's owner) and the
// run mode to direct so the test does not depend on the host's systemd session.
func TestResolveRuntime_NamespaceFromSettings(t *testing.T) {
	writeRuntimeConfig(t, "containerd:\n  namespace: k8s.io\n")
	t.Setenv("CHARLY_RUN_ENGINE", "containerd")
	t.Setenv("CHARLY_RUN_MODE", "direct")

	rt, err := ResolveRuntime()
	if err != nil {
		t.Fatalf("ResolveRuntime: %v", err)
	}
	if rt.Namespace != "k8s.io" {
		t.Errorf("Namespace = %q, want %q (the settings value must win)", rt.Namespace, "k8s.io")
	}
}

// TestResolveRuntime_NamespaceDefaultsFromCapability pins the built-in level:
// with no settings value, the namespace comes from the resolved engine's
// capability row (containerd's `namespace` fact is "default"), not from a
// consumer-side literal.
func TestResolveRuntime_NamespaceDefaultsFromCapability(t *testing.T) {
	writeRuntimeConfig(t, "")
	t.Setenv("CHARLY_RUN_ENGINE", "containerd")
	t.Setenv("CHARLY_RUN_MODE", "direct")

	rt, err := ResolveRuntime()
	if err != nil {
		t.Fatalf("ResolveRuntime: %v", err)
	}
	if rt.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q from the containerd capability row", rt.Namespace, "default")
	}
}

// TestResolveRuntime_NamespaceHasNoEnvMirror pins the plan's table: the
// namespace row has NO env column, so an env var that looks plausible must not
// silently become a precedence level. Only the settings value and the
// capability default may produce the resolved namespace.
func TestResolveRuntime_NamespaceHasNoEnvMirror(t *testing.T) {
	writeRuntimeConfig(t, "")
	t.Setenv("CHARLY_RUN_ENGINE", "containerd")
	t.Setenv("CHARLY_RUN_MODE", "direct")
	t.Setenv("CHARLY_CONTAINERD_NAMESPACE", "from-env")

	rt, err := ResolveRuntime()
	if err != nil {
		t.Fatalf("ResolveRuntime: %v", err)
	}
	if rt.Namespace != "default" {
		t.Errorf("Namespace = %q; the namespace has no env mirror, want the capability default %q", rt.Namespace, "default")
	}
}
