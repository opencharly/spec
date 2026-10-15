package exec

// The LIVE leg of the plugin delivery: the same function the deploy calls, run against a REAL
// spec.DeployExecutor (ShellExecutor — `bash -c` on this machine) rather than an in-memory stand-in.
// This is the difference between a unit proof and `analysed on a live system`: the delivery's own
// commands (mkdir, PutFile, the manifest count, the symlink) execute for real, and the tree is read
// back off the filesystem afterwards.
//
// It is NOT an R10 bed: no venue is provisioned and no charly is deployed. It proves the delivery
// mechanics end to end on a real executor, and it is deliberately honest about what it does not
// prove — that a plugin verb then WORKS in a guest, which needs a bed.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeliverPluginsToARealExecutor(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no bash available for ShellExecutor")
	}

	// A tree that is genuinely a plugin tree: provider binaries beside their manifests.
	src := t.TempDir()
	for _, name := range []string{"plugin-deploy-firecracker", "plugin-firecracker"} {
		bin := filepath.Join(src, name)
		if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin+ManifestSuffix, []byte("deploy:firecracker\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	ve := &ShellExecutor{}

	remote, err := EnsurePluginsInDeployVenue(ctx, ve, src, "2026.288.0001")
	if err != nil {
		t.Fatalf("live delivery failed: %v", err)
	}
	// The replica lives under /tmp and is content-addressed; it is removed at the end so the test
	// does not accumulate trees across runs.
	defer os.RemoveAll(remote)
	defer os.Remove(StablePluginsPath)

	// Read the delivered tree back OFF THE FILESYSTEM — not from the function's return value.
	for _, name := range []string{"plugin-deploy-firecracker", "plugin-firecracker"} {
		info, err := os.Stat(filepath.Join(remote, name))
		if err != nil {
			t.Fatalf("the delivery reported success but %s is not on the venue: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s arrived without its executable bit (mode %v) — the loader cannot serve a provider it cannot run", name, info.Mode().Perm())
		}
		if _, err := os.Stat(filepath.Join(remote, name+ManifestSuffix)); err != nil {
			t.Fatalf("%s%s did not arrive, so the word it serves is not declared: %v", name, ManifestSuffix, err)
		}
	}

	// And the published name the PLAN emits must point at that tree.
	target, err := os.Readlink(StablePluginsPath)
	if err != nil {
		t.Fatalf("the stable path %s is not a symlink after a live delivery: %v", StablePluginsPath, err)
	}
	if target != remote {
		t.Fatalf("%s -> %s, want the delivered replica %s", StablePluginsPath, target, remote)
	}

	// The idempotence contract, measured live: a second delivery of the same tree reuses the replica
	// instead of re-streaming it.
	again, err := EnsurePluginsInDeployVenue(ctx, ve, src, "2026.288.0001")
	if err != nil {
		t.Fatalf("second live delivery failed: %v", err)
	}
	if again != remote {
		t.Fatalf("an unchanged tree must reuse its replica: %s then %s", remote, again)
	}
}
