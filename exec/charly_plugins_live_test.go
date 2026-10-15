package exec

// The LIVE legs of the plugin delivery — the same function the deploy calls, run
// against a REAL spec.DeployExecutor rather than an in-memory stand-in: the delivery's
// own commands (mkdir, PutFile, the manifest count, the symlink) execute for real, and
// the tree is read back OFF THE FILESYSTEM afterwards.
//
// TWO callers, ONE implementation of "deliver and read the boundary back"
// (deliverAndReadBack), because the two legs differ only in WHERE the built tree comes
// from:
//
//   - TestDeliverPluginsToARealExecutor is the repo's own leg. `go test ./exec/` is
//     green on a bare checkout with no venue, so it makes its own tree.
//   - TestDeliverTheBuiltTreeIntoTheDeployVenue is the check-spec-delivery BED's leg.
//     The bed assembles the BUILT plugin tree on the venue and hands its path over.
//
// Neither is the R10 gate by itself. The gate is the bed: this leg does the delivery,
// and the bed's OWN next step then dispatches the delivered word through the venue's
// ACTIVE charly, so the served RESULT is asserted at the boundary a deploy consumes —
// not here, and not by reading a return value.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// venueExecutor is the executor a `host: local` deploy actually runs its venue through
// — `RootExecutorForDeployNode` returns ShellExecutor{MachineVenue: true} for host
// "local" (deploy_chain.go). MachineVenue is the part that matters here and is easy to
// drop silently: it is what prefixes the venue user's env.d preamble onto a
// non-interactive step, i.e. what a real deploy's own scripts get and a throwaway
// `ShellExecutor{}` does not.
func venueExecutor() spec.DeployExecutor { return ShellExecutor{MachineVenue: true} }

// deliverAndReadBack is THE one implementation of the delivery's boundary contract:
// hand a built plugin tree to the function under test, run it over the venue's own
// executor, then read the result back OFF THE VENUE — never from the function's return
// value, which is a claim about the venue and not the venue itself. It returns the
// content-addressed replica path the delivery chose.
func deliverAndReadBack(t *testing.T, tree, hostVer, stablePath string) string {
	t.Helper()

	want, err := CountManifests(tree)
	if err != nil {
		t.Fatal(err)
	}
	if want == 0 {
		t.Fatalf("%s carries no %s manifest, so it is not a plugin tree", tree, ManifestSuffix)
	}
	bins := pluginBinaries(t, tree)

	remote, err := EnsurePluginsInDeployVenueAt(context.Background(), venueExecutor(), tree, hostVer, stablePath)
	if err != nil {
		t.Fatalf("live delivery of %s into the venue: %v", tree, err)
	}

	// Read the delivered tree back OFF THE VENUE's filesystem, and require exactly the
	// manifest count the source carried: a delivery that landed half the tree is not a
	// delivery, and only a count read from the destination can tell the two apart.
	got, err := CountManifests(remote)
	if err != nil {
		t.Fatalf("reading the delivered tree back from %s: %v", remote, err)
	}
	if got != want {
		t.Fatalf("the venue carries %d %s manifest(s) at %s, want the %d the source carried", got, ManifestSuffix, remote, want)
	}
	for _, bin := range bins {
		rel, rerr := filepath.Rel(tree, bin)
		if rerr != nil {
			t.Fatal(rerr)
		}
		dst := filepath.Join(remote, rel)
		info, serr := os.Stat(dst)
		if serr != nil {
			t.Fatalf("the delivery reported success but %s is not on the venue: %v", rel, serr)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s arrived without its executable bit (mode %v) — the loader cannot serve a provider it cannot run", rel, info.Mode().Perm())
		}
	}

	// The stable name the PLAN emits must point at that replica. This is the one link
	// the two halves of the contract meet at: a plan is compiled host-side BEFORE any
	// delivery, so it cannot name a digest, and the delivery cannot know a plan will be
	// applied — so the published name is the only thing that can join them.
	target, err := os.Readlink(stablePluginsPath(stablePath))
	if err != nil {
		t.Fatalf("the stable path %s is not a symlink after a live delivery: %v", stablePluginsPath(stablePath), err)
	}
	if target != remote {
		t.Fatalf("%s -> %s, want the delivered replica %s", stablePluginsPath(stablePath), target, remote)
	}
	return remote
}

// pluginBinaries walks a plugin tree and returns the provider binary each
// `<name>.providers` manifest names. A manifest with no executable beside it is a
// declared capability with nothing behind it, so it is refused HERE rather than at
// connect time on the venue.
func pluginBinaries(t *testing.T, tree string) []string {
	t.Helper()
	var bins []string
	err := filepath.Walk(tree, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() || filepath.Ext(path) != ManifestSuffix {
			return nil
		}
		bin := strings.TrimSuffix(path, ManifestSuffix)
		st, serr := os.Stat(bin)
		if serr != nil {
			return fmt.Errorf("manifest %s declares capabilities with no provider binary beside it: %w", path, serr)
		}
		if st.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("provider %s is not executable (mode %v)", bin, st.Mode().Perm())
		}
		bins = append(bins, bin)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bins
}

// TestDeliverPluginsToARealExecutor is the repo's own live leg: the delivery runs for
// real against ShellExecutor on a bare checkout that has no venue and no built plugin
// tree, so it makes its own. It proves the delivery MECHANICS end to end; it does not
// prove that a venue serves a word afterwards — the bed does that, and says so.
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

	// One private stable path for BOTH deliveries: the shared /tmp/charly-plugins is never touched, and
	// sharing the path is what lets the warm re-delivery's republish be asserted below.
	stable := filepath.Join(t.TempDir(), "charly-plugins")
	remote := deliverAndReadBack(t, src, "2026.288.0001", stable)

	// Take this leg's replica away again. Registered right after the FIRST delivery, so a failure
	// later in the test still cleans up. No link cleanup is needed anywhere: both deliveries publish
	// to a t.TempDir() path, so the shared StablePluginsPath is never touched and never has to be restored.
	defer func() { _ = os.RemoveAll(remote) }()

	// The idempotence contract, measured live: a second delivery of the same tree reuses
	// the replica instead of re-streaming it.
	// Remove the link the FIRST delivery published, so the warm re-delivery below has to republish it.
	// Sharing one path is not enough: without this, the first delivery's link would satisfy the readback
	// whether or not the warm branch publishes, and the assertion could not fail.
	if err := os.Remove(stable); err != nil {
		t.Fatalf("removing the link the first delivery published at %s: %v", stable, err)
	}

	again, err := EnsurePluginsInDeployVenueAt(context.Background(), venueExecutor(), src, "2026.288.0001", stable)
	if err != nil {
		t.Fatalf("second live delivery failed: %v", err)
	}
	if again != remote {
		t.Fatalf("an unchanged tree must reuse its replica: %s then %s", remote, again)
	}

	// And the warm re-delivery must have REPUBLISHED the stable name. Without this readback the
	// assertion above passes whether or not the warm branch publishes, because the replica path is
	// content-derived either way — so removing the republish would go unnoticed.

	if target, lerr := os.Readlink(stable); lerr != nil || target != remote {
		t.Fatalf("the warm re-delivery must republish %s -> %s, got %q (%v)", stable, remote, target, lerr)
	}

}

// TestDeliverTheBuiltTreeIntoTheDeployVenue is the check-spec-delivery BED's delivery
// leg. The bed assembles the BUILT plugin tree on the venue — the provider binary for
// the plugin candy that venue's own composition declares, beside the `.providers`
// manifest that candy declares — and hands over its path in SPEC_DELIVERY_TREE, plus
// the ACTIVE binary's CalVer in SPEC_DELIVERY_HOSTVER (the same version token a deploy
// passes, and the key the content-addressed replica is named with).
//
// The replica and the published link are DELIBERATELY left in place: the bed's next step
// serves the word out of the published path, and this leg removing them would be it
// cleaning up the very artefact it exists to produce.
//
// It skips when the bed's inputs are absent — a bare `go test ./exec/` has no built tree
// and no venue, so there is nothing to deliver into. A skip here can never read as a
// pass: the bed's next step fails outright when this leg published nothing.
func TestDeliverTheBuiltTreeIntoTheDeployVenue(t *testing.T) {
	tree := os.Getenv("SPEC_DELIVERY_TREE")
	hostVer := os.Getenv("SPEC_DELIVERY_HOSTVER")
	if tree == "" || hostVer == "" {
		t.Skip("SPEC_DELIVERY_TREE and SPEC_DELIVERY_HOSTVER are set only by the check-spec-delivery bed's step, which is where a provisioned venue exists to deliver into")
	}

	remote := deliverAndReadBack(t, tree, hostVer, "")
	t.Logf("the built plugin tree from %s is on the venue at %s, published at %s", tree, remote, StablePluginsPath)
}
