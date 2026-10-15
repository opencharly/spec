package exec

// The plugin-delivery gates. R7 requires coverage that FAILS without the change: the empty-tree
// refusal is the case that would otherwise let a "fix" ship #916's symptom unchanged, and the
// digest is what makes the delivery idempotent instead of re-streaming a tree every deploy.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// writeTree materialises a plugin tree: provider binaries with their `<name>.providers` manifests.
func writeTree(t *testing.T, manifests ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range manifests {
		bin := filepath.Join(dir, name)
		if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin+ManifestSuffix, []byte("deploy:firecracker\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCountManifestsCountsTheServedSide(t *testing.T) {
	tree := writeTree(t, "plugin-deploy-firecracker", "plugin-firecracker")
	n, err := CountManifests(tree)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("CountManifests = %d, want 2 (one per provider binary)", n)
	}

	empty := t.TempDir()
	n, err = CountManifests(empty)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("CountManifests(empty) = %d, want 0", n)
	}
}

// TestEnsurePluginsRefusesATreeWithoutManifests is the control for #916's own failure mode: a tree
// carrying no manifest is empty of capability, and delivering it would leave the venue exactly as
// broken as before while the deploy reported success.
func TestEnsurePluginsRefusesATreeWithoutManifests(t *testing.T) {
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "some-binary"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Asserted on the PURE predicate, not through EnsurePluginsInDeployVenue: that function guards
	// its nil executor first, so a call with nil would fail for the wrong reason and the control
	// would pass with this refusal deleted — measured, on the first version of this test.
	if err := ValidatePluginTree(empty); err == nil {
		t.Fatal("a tree with no .providers manifest must be refused, not delivered")
	}
	// And the delivery path must refuse it too, for the same reason and before any venue call.
	if _, err := EnsurePluginsInDeployVenue(context.Background(), nil, empty, "2026.288.0001"); err == nil {
		t.Fatal("the delivery must refuse an empty plugin tree")
	}
	// The positive arm: a real tree passes the same predicate.
	if err := ValidatePluginTree(writeTree(t, "plugin-deploy-firecracker")); err != nil {
		t.Fatalf("a tree carrying a manifest must validate: %v", err)
	}
}

// TestReplicatedPluginsPathIsContentAddressed pins the idempotence contract: the path changes when
// the served bytes change and is stable when they do not, because it is keyed by the tree digest.
func TestReplicatedPluginsPathIsContentAddressed(t *testing.T) {
	ver := "2026.288.0001"
	tree := writeTree(t, "plugin-deploy-firecracker")

	first, err := replicatedPluginsPath(tree, ver)
	if err != nil {
		t.Fatal(err)
	}
	again, err := replicatedPluginsPath(tree, ver)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatalf("an unchanged tree must resolve to one path: %s vs %s", first, again)
	}
	if want := "/tmp/charly-plugins-" + ver + "-"; len(first) <= len(want) || first[:len(want)] != want {
		t.Fatalf("path %q does not carry the versioned, content-addressed prefix %q", first, want)
	}

	// Change the served bytes: the path must move, or a stale tree would be reused forever.
	if err := os.WriteFile(filepath.Join(tree, "plugin-deploy-firecracker"+ManifestSuffix),
		[]byte("deploy:firecracker\ndeploy:notreal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := replicatedPluginsPath(tree, ver)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatalf("a changed manifest must change the replica path, both were %s", first)
	}
}

// TestTarPluginTreeRoundTripsWithModes pins the guest delivery's transport: the tree must survive
// tar/gzip with its MODES, because a provider binary that arrives non-executable is a plugin the
// loader cannot serve — the delivery would land and be useless.
func TestTarPluginTreeRoundTripsWithModes(t *testing.T) {
	tree := writeTree(t, "plugin-deploy-firecracker")

	archive, err := tarPluginTree(tree)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(archive)

	dest := t.TempDir()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dest, hdr.Name)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, nil, os.FileMode(hdr.Mode)); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{"plugin-deploy-firecracker", "plugin-deploy-firecracker" + ManifestSuffix} {
		info, err := os.Stat(filepath.Join(dest, name))
		if err != nil {
			t.Fatalf("%s did not survive the archive: %v", name, err)
		}
		src, err := os.Stat(filepath.Join(tree, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != src.Mode().Perm() {
			t.Fatalf("%s arrived with mode %v, want %v", name, info.Mode().Perm(), src.Mode().Perm())
		}
	}

	// The round-tripped tree is still a plugin tree by the same predicate the delivery enforces.
	if err := ValidatePluginTree(dest); err != nil {
		t.Fatalf("the extracted tree must validate: %v", err)
	}
}
