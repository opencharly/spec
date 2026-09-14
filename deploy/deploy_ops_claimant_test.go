package deploy

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// deploy_ops_claimant_test.go — FindVMClaimant identity scoping.
//
// Regression: MANY deploys routinely share one kind:vm entity (a 16-bed suite all
// `from: omarchy-vm`), and exactly one may carry `requires_exclusive`. The former
// entity-wide scan made EVERY sibling inherit that claim — a GPU-free bed
// (`check-omarchy-iso-vm`) failed `vm create` demanding an NVIDIA card that only
// the hybrid-GPU bed's host has. These cases pin the fix.

func vmNode(from string, excl ...string) spec.DeployNode {
	n := spec.DeployNode{From: from}
	n.Descent = &spec.DescentDescriptor{Venue: "ssh", Transport: "ssh"}
	n.RequiresExclusive = excl
	return n
}

// The bug: with a claimant identity, the resolve returns THAT node, never a
// sibling sharing the entity.
func TestFindVMClaimant_IdentityScoped(t *testing.T) {
	tree := map[string]spec.DeployNode{
		"check-omarchy-hybrid-gpu-vm": vmNode("omarchy-vm", "nvidia-gpu"),
		"check-omarchy-iso-vm":        vmNode("omarchy-vm"),
	}
	// The iso bed claims NO exclusive resource → no claimant.
	if name, _, ok := FindVMClaimant(tree, "omarchy-vm", "check-omarchy-iso-vm"); ok {
		t.Fatalf("the GPU-free iso bed must resolve no claimant, got %q", name)
	}
	// The hybrid bed resolves to ITSELF.
	name, _, ok := FindVMClaimant(tree, "omarchy-vm", "check-omarchy-hybrid-gpu-vm")
	if !ok || name != "check-omarchy-hybrid-gpu-vm" {
		t.Fatalf("the hybrid bed must resolve its own claim, got name=%q ok=%v", name, ok)
	}
}

// A domain identity (VmDomainIdentity form) resolves the same node as its deploy key.
func TestFindVMClaimant_IdentityNormalized(t *testing.T) {
	tree := map[string]spec.DeployNode{
		"vm:prod/hybrid": vmNode("base", "nvidia-gpu"),
	}
	// VmDomainIdentity("vm:prod/hybrid") == "prod-hybrid".
	if name, _, ok := FindVMClaimant(tree, "base", "prod-hybrid"); !ok || name != "vm:prod/hybrid" {
		t.Fatalf("domain-identity form must resolve, got name=%q ok=%v", name, ok)
	}
}

// The legacy no-identity path: a SINGLE exclusive claimant resolves (the direct
// `charly vm create <entity>` case).
func TestFindVMClaimant_NoIdentitySingle(t *testing.T) {
	tree := map[string]spec.DeployNode{
		"only-gpu-bed": vmNode("omarchy-vm", "nvidia-gpu"),
		"plain-bed":    vmNode("omarchy-vm"),
	}
	name, _, ok := FindVMClaimant(tree, "omarchy-vm", "")
	if !ok || name != "only-gpu-bed" {
		t.Fatalf("single-claimant legacy resolve got name=%q ok=%v", name, ok)
	}
}

// The legacy no-identity path is AMBIGUOUS with >1 exclusive claimant: return
// false rather than an arbitrary map-order winner (which is how the bug surfaced).
func TestFindVMClaimant_NoIdentityAmbiguous(t *testing.T) {
	tree := map[string]spec.DeployNode{
		"gpu-bed-a": vmNode("omarchy-vm", "nvidia-gpu"),
		"gpu-bed-b": vmNode("omarchy-vm", "nvidia-gpu"),
	}
	if name, _, ok := FindVMClaimant(tree, "omarchy-vm", ""); ok {
		t.Fatalf("ambiguous multi-claimant must not resolve, got %q", name)
	}
}
