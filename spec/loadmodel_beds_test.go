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
