package spec

// loadmodel_beds_test.go — coverage for the ONE bed resolver
// (UnifiedFile.Beds / ResolveBed / BedScope), the namespace-aware replacement for
// the former local-only CheckBeds(). Locks the fix for the namespaced-bed
// validate-but-not-run defect.

import (
	"sort"
	"testing"
)

func disposable() *bool { b := true; return &b }

// bedFold builds a root file with a local disposable bed and an `omarchy`
// namespace carrying its own disposable bed, plus a non-disposable deploy.
func bedFold() *UnifiedFile {
	ns := &UnifiedFile{
		Deploy: map[string]DeployNode{
			"ns-bed":    {From: "ns-vm", Disposable: disposable()},
			"ns-nonbed": {From: "ns-vm"},
			"ns-member": {From: "ns-vm", Disposable: disposable(), MemberOf: "ns-bed"},
		},
	}
	return &UnifiedFile{
		Deploy: map[string]DeployNode{
			"local-bed":    {From: "local-vm", Disposable: disposable()},
			"local-nonbed": {From: "local-vm"},
		},
		Namespaces: map[string]*UnifiedFile{"omarchy": ns},
	}
}

func TestBeds_NamespaceAware(t *testing.T) {
	uf := bedFold()
	beds := uf.Beds()
	got := make([]string, 0, len(beds))
	for k := range beds {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"local-bed", "omarchy.ns-bed"}
	if len(got) != len(want) {
		t.Fatalf("Beds() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Beds() = %v, want %v", got, want)
		}
	}
	// A member (MemberOf != "") and a non-disposable deploy are never beds.
	for _, notBed := range []string{"omarchy.ns-nonbed", "omarchy.ns-member", "local-nonbed"} {
		if _, ok := uf.Beds()[notBed]; ok {
			t.Fatalf("Beds() included non-bed %q", notBed)
		}
	}
}

func TestResolveBed_Forms(t *testing.T) {
	uf := bedFold()
	cases := []struct {
		ref  string
		want bool
	}{
		{"local-bed", true},
		{"omarchy.ns-bed", true},
		{"ns-bed", false}, // unqualified never reaches a namespace
		{"omarchy.ns-nonbed", false},
		{"missing", false},
		{"", false},
	}
	for _, c := range cases {
		if _, ok := uf.ResolveBed(c.ref); ok != c.want {
			t.Fatalf("ResolveBed(%q) ok = %v, want %v", c.ref, ok, c.want)
		}
	}
	// The resolved node is the real deploy.
	if n, ok := uf.ResolveBed("omarchy.ns-bed"); !ok || n.From != "ns-vm" {
		t.Fatalf("ResolveBed(omarchy.ns-bed) = (%+v, %v), want From=ns-vm", n, ok)
	}
}

// TestBedResolveForRoot_QualifiesNamespacedRefs is the regression for the
// namespaced-bed box-build defect: a bed run drives `charly box build <image>` /
// `charly deploy add <name> <image>` / `charly vm build <from>` from the project
// ROOT, where a namespaced bed's bare image:/from: does NOT resolve (measured live:
// `charly.check-sidecar-pod` failed with `unknown box "check-k8s-deploy-app"`).
// ResolveBedForRoot rewrites them to ns.<leaf>, recursively through members.
func TestBedResolveForRoot_QualifiesNamespacedRefs(t *testing.T) {
	ns := &UnifiedFile{
		Deploy: map[string]DeployNode{
			"ns-pod": {Image: "ns-app", Disposable: disposable(),
				Member: []Member{{Name: "m", Position: PositionInSubstrate,
					Node: &Deploy{Image: "ns-app", From: "ns-vm"}}}},
			"ns-vm-bed": {From: "ns-vm", Disposable: disposable()},
		},
	}
	root := &UnifiedFile{
		Deploy: map[string]DeployNode{
			"local-pod": {Image: "root-app", Disposable: disposable()},
			// An already-qualified ref must NOT be double-prefixed, and a local one stays.
			"local-mixed": {Image: "other.thing", From: "local-vm", Disposable: disposable()},
		},
		Namespaces: map[string]*UnifiedFile{"omarchy": ns},
	}

	// Namespaced pod bed: image + member image/from qualify.
	n, ok := root.ResolveBedForRoot("omarchy.ns-pod")
	if !ok {
		t.Fatal("omarchy.ns-pod did not resolve")
	}
	if n.Image != "omarchy.ns-app" {
		t.Fatalf("image = %q, want omarchy.ns-app", n.Image)
	}
	if len(n.Member) != 1 || n.Member[0].Node.Image != "omarchy.ns-app" || n.Member[0].Node.From != "omarchy.ns-vm" {
		t.Fatalf("member refs not qualified: %+v", n.Member)
	}
	// The stored tree is unmutated (validate reads the authored refs) — including
	// every member node, which carries a *Deploy pointer into the stored tree.
	if ns.Deploy["ns-pod"].Image != "ns-app" {
		t.Fatal("ResolveBedForRoot mutated the stored Deploy tree (root image)")
	}
	stored := ns.Deploy["ns-pod"]
	if len(stored.Member) != 1 || stored.Member[0].Node == nil ||
		stored.Member[0].Node.Image != "ns-app" || stored.Member[0].Node.From != "ns-vm" {
		t.Fatalf("ResolveBedForRoot mutated the stored member node: %+v", stored.Member)
	}

	// Namespaced vm bed: from qualifies.
	if n, ok := root.ResolveBedForRoot("omarchy.ns-vm-bed"); !ok || n.From != "omarchy.ns-vm" {
		t.Fatalf("omarchy.ns-vm-bed From = %q, want omarchy.ns-vm", n.From)
	}

	// A local bed is returned unchanged.
	if n, ok := root.ResolveBedForRoot("local-pod"); !ok || n.Image != "root-app" {
		t.Fatalf("local-pod image = %q, want root-app (unchanged)", n.Image)
	}
	// An already-qualified ref is never double-prefixed; a local from: stays.
	if n, ok := root.ResolveBedForRoot("local-mixed"); !ok || n.Image != "other.thing" || n.From != "local-vm" {
		t.Fatalf("local-mixed = (%q,%q), want (other.thing, local-vm) unchanged", n.Image, n.From)
	}
}

func TestBedScope_OwningNamespace(t *testing.T) {
	uf := bedFold()
	scope, leaf := uf.BedScope("omarchy.ns-bed")
	if scope != uf.Namespaces["omarchy"] || leaf != "ns-bed" {
		t.Fatalf("BedScope(omarchy.ns-bed) = (%v, %q), want (omarchy namespace, ns-bed)", scope, leaf)
	}
	scope, leaf = uf.BedScope("local-bed")
	if scope != uf || leaf != "local-bed" {
		t.Fatalf("BedScope(local-bed) = (%v, %q), want (root, local-bed)", scope, leaf)
	}
	if scope, _ := uf.BedScope("missing"); scope != nil {
		t.Fatalf("BedScope(missing) = %v, want nil", scope)
	}
}

// TestBeds_MutualCycleTerminates: a mutual import (main imports sub, sub imports
// main) makes the Namespaces graph cyclic; Beds() must terminate (an unguarded
// walk loops forever; a global visited set would drop a shared multi-alias mount).
func TestBeds_MutualCycleTerminates(t *testing.T) {
	main := &UnifiedFile{}
	sub := &UnifiedFile{Deploy: map[string]DeployNode{
		"sub-bed": {From: "x", Disposable: disposable()},
	}}
	main.Deploy = map[string]DeployNode{"main-bed": {From: "y", Disposable: disposable()}}
	main.Namespaces = map[string]*UnifiedFile{"sub": sub}
	sub.Namespaces = map[string]*UnifiedFile{"up": main} // mutual cycle

	beds := main.Beds()
	if _, ok := beds["main-bed"]; !ok {
		t.Fatal("local bed missing")
	}
	if _, ok := beds["sub.sub-bed"]; !ok {
		t.Fatal("nested bed missing")
	}
	if _, ok := beds["sub.up.main-bed"]; ok {
		t.Fatal("mutual-cycle back-edge was not skipped")
	}
}

// TestDeploys_NamespaceQualified pins the namespace-aware deploy fold: a merged-root consumer
// resolving `charly.check-agentteams-vm` needs the qualified key, not the root-scope-only raw map.
func TestDeploys_NamespaceQualified(t *testing.T) {
	ns := &UnifiedFile{Deploy: map[string]DeployNode{"ns-vm": {From: "base-vm", Target: "vm"}}}
	root := &UnifiedFile{
		Deploy:     map[string]DeployNode{"local-pod": {Image: "x", Target: "pod"}},
		Namespaces: map[string]*UnifiedFile{"charly": ns},
	}
	d := root.Deploys()
	if _, ok := d["local-pod"]; !ok {
		t.Fatal("local deploy missing")
	}
	if _, ok := d["charly.ns-vm"]; !ok {
		t.Fatalf("namespaced deploy missing; keys = %v", d)
	}
}

// TestDeploys_QualifiesNamespacedFrom pins that a namespaced deploy's `from:` is root-qualified in
// the fold (so plugin-deploy-vm's prepare-venue, which resolves `from:` as a kind:vm entity, finds
// it), while its `image:` is left UNqualified (it doubles as an OCI base ref whose leaf the pod
// overlay resolves).
func TestDeploys_QualifiesNamespacedFrom(t *testing.T) {
	ns := &UnifiedFile{Deploy: map[string]DeployNode{
		"check-vm":  {From: "base-vm", Target: "vm"},
		"check-pod": {Image: "docs-site-app", Target: "pod"},
	}}
	root := &UnifiedFile{Namespaces: map[string]*UnifiedFile{"charly": ns}}
	d := root.Deploys()
	if got := d["charly.check-vm"].From; got != "charly.base-vm" {
		t.Fatalf("namespaced from = %q, want charly.base-vm (must be root-qualified)", got)
	}
	if got := d["charly.check-pod"].Image; got != "docs-site-app" {
		t.Fatalf("namespaced image = %q, want docs-site-app (must stay UNqualified)", got)
	}
}
