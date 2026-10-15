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

// preserveStablePluginsPath records the CURRENT target of StablePluginsPath and restores it
// when the test ends — the invariant on the SURVIVOR, not on the change.
//
// EnsurePluginsInDeployVenue publishes StablePluginsPath unconditionally, and on the venue a
// `host: local` deploy actually uses (ShellExecutor{MachineVenue: true}, what venueExecutor
// returns) that path is the developer's own shared /tmp/charly-plugins. The delivery's own
// deferred cleanup removes the link only when it still points at this leg's replica; it never
// restores what it replaced. So without this, `go test ./exec/` repoints a shared path on the
// machine it runs on, and a second run cannot tell that from a fresh machine.
func preserveStablePluginsPath(t *testing.T) {
	t.Helper()
	prior, err := os.Readlink(StablePluginsPath)
	had := err == nil
	t.Cleanup(func() {
		t.Helper()
		if err := os.Remove(StablePluginsPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("clearing %s during restore: %v", StablePluginsPath, err)
			return
		}
		if had {
			if err := os.Symlink(prior, StablePluginsPath); err != nil {
				t.Errorf("restoring %s -> %s: %v", StablePluginsPath, prior, err)
				return
			}
		}
		// The invariant: whatever the delivery did, the survivor is what it was.
		now, err := os.Readlink(StablePluginsPath)
		if had && (err != nil || now != prior) {
			t.Errorf("%s survived as (%q, %v), want %q", StablePluginsPath, now, err, prior)
		}
		if !had && err == nil {
			t.Errorf("%s survived as %q, want it absent", StablePluginsPath, now)
		}
	})
}

// deliverAndReadBack is THE one implementation of the delivery's boundary contract:
// hand a built plugin tree to the function under test, run it over the venue's own
// executor, then read the result back OFF THE VENUE — never from the function's return
// value, which is a claim about the venue and not the venue itself. It returns the
// content-addressed replica path the delivery chose.
func deliverAndReadBack(t *testing.T, tree, hostVer string) string {
	t.Helper()

	want, err := CountManifests(tree)
	if err != nil {
		t.Fatal(err)
	}
	if want == 0 {
		t.Fatalf("%s carries no %s manifest, so it is not a plugin tree", tree, ManifestSuffix)
	}
	bins := pluginBinaries(t, tree)

	preserveStablePluginsPath(t)
	remote, err := EnsurePluginsInDeployVenue(context.Background(), venueExecutor(), tree, hostVer)
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
	target, err := os.Readlink(StablePluginsPath)
	if err != nil {
		t.Fatalf("the stable path %s is not a symlink after a live delivery: %v", StablePluginsPath, err)
	}
	if target != remote {
		t.Fatalf("%s -> %s, want the delivered replica %s", StablePluginsPath, target, remote)
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

	remote := deliverAndReadBack(t, src, "2026.288.0001")

	// The idempotence contract, measured live: a second delivery of the same tree reuses
	// the replica instead of re-streaming it.
	preserveStablePluginsPath(t)
	again, err := EnsurePluginsInDeployVenue(context.Background(), venueExecutor(), src, "2026.288.0001")
	if err != nil {
		t.Fatalf("second live delivery failed: %v", err)
	}
	if again != remote {
		t.Fatalf("an unchanged tree must reuse its replica: %s then %s", remote, again)
	}

	// Take this leg's replica away again — this tree lives under t.TempDir() and nothing
	// else can be serving it. The published link goes only if it is still THIS leg's:
	// removing a link another leg published would be this test cleaning up work it does
	// not own.
	defer func() { _ = os.RemoveAll(remote) }()
	if target, lerr := os.Readlink(StablePluginsPath); lerr == nil && target == remote {
		defer func() { _ = os.Remove(StablePluginsPath) }()
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

	remote := deliverAndReadBack(t, tree, hostVer)
	t.Logf("the built plugin tree from %s is on the venue at %s, published at %s", tree, remote, StablePluginsPath)
}
