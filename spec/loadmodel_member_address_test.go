package spec

import "testing"

// TestDeploys_FoldedDeployLevelMemberKeepsBareAddress is the merged-root projection half of
// opencharly/charly#1083's regression.
//
// THE LAW: a deploy-level member is a top-level addressable entry addressed by its BARE name
// (Cutover C; sdk/loaderkit.FoldMembers promotes it into its owner's scope). The projection must
// therefore emit a FOLDED member (MemberOf set) under its bare name EVEN when it was folded inside
// an imported namespace — the namespace prefix qualifies namespace-scoped entities, not a member
// whose bare address is global. Its `from:` cross-ref IS namespace-local, so it stays qualified, or
// the re-entrant `charly deploy add <member>` resolves the member but not its VM entity.
//
// WITHOUT the rule the only emitted key is `charly.check-sidecar-pod-ephvm`, so the child dispatch
// (which is handed the bare `check-sidecar-pod-ephvm`) resolves no node and defaults the substrate
// to pod.
func TestDeploys_FoldedDeployLevelMemberKeepsBareAddress(t *testing.T) {
	ns := &UnifiedFile{
		Deploy: map[string]DeployNode{
			"check-sidecar-pod": {
				Target: "pod",
				Member: []Member{{
					Name:     "check-sidecar-pod-ephvm",
					Position: PositionDeployLevel,
					Node:     &Deploy{Target: "vm", From: "eval-vm"},
				}},
			},
			// The FOLDED entry (what FoldMembers produces once it recurses namespaces).
			"check-sidecar-pod-ephvm": {
				Target:   "vm",
				From:     "eval-vm",
				MemberOf: "check-sidecar-pod",
			},
		},
	}
	root := &UnifiedFile{Namespaces: map[string]*UnifiedFile{"charly": ns}}

	got := root.Deploys()
	n, ok := got["check-sidecar-pod-ephvm"]
	if !ok {
		t.Fatalf("Deploys() has no BARE key for the folded namespaced member (keys: %v)\n"+
			"  This is opencharly/charly#1083: the member is addressable by its bare name by law, "+
			"so `charly deploy add check-sidecar-pod-ephvm` must resolve this node.",
			DeployKeys(got))
	}
	if n.Target != "vm" {
		t.Errorf("folded member Target = %q, want vm", n.Target)
	}
	if n.From != "charly.eval-vm" {
		t.Errorf("folded member From = %q, want the root-qualified charly.eval-vm", n.From)
	}
	// The owner itself keeps its qualified address; the bare key is the MEMBER's alone.
	if _, ok := got["charly.check-sidecar-pod"]; !ok {
		t.Errorf("namespace-scoped top-level deploy lost its qualified key (keys: %v)", DeployKeys(got))
	}
}

// DeployKeys is a tiny test-local view of a deploy map's keys, so a failing assertion names the
// whole key set instead of only the miss.
func DeployKeys(m map[string]DeployNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
