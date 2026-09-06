package exec

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestResolveDeployChainUniformMemberTree pins the executor-chain walk over the
// ONE uniform ordered member tree (Cutover C task 0): a dotted path resolves
// through member entries of EITHER position — the chain stacks one venue hop per
// segment exactly as the dual Children map did for the in-substrate class.
func TestResolveDeployChainUniformMemberTree(t *testing.T) {
	container := func() *spec.DescentDescriptor {
		return &spec.DescentDescriptor{Transport: "container-exec"}
	}
	roots := map[string]spec.FleetNode{
		"web": {
			Target:  "pod",
			Descent: container(),
			Member: []spec.Member{
				// Authored order non-alphabetical: order must not matter to the
				// dotted-path lookup.
				{Name: "zulu", Position: spec.PositionDeployLevel, Node: &spec.FleetNode{Target: "vm", Descent: &spec.DescentDescriptor{Transport: "ssh"}}},
				{Name: "db", Position: spec.PositionInSubstrate, Node: &spec.FleetNode{Target: "pod", Descent: container()}},
			},
		},
	}

	node, chain, err := ResolveDeployChain(roots, "web.db", ShellExecutor{})
	if err != nil {
		t.Fatalf("ResolveDeployChain(web.db): %v", err)
	}
	if node.Target != "pod" {
		t.Fatalf("leaf target = %q, want pod", node.Target)
	}
	// Two container-exec hops: charly-web, then charly-web_db.
	hops := executorHops(chain)
	if len(hops) != 2 || hops[0] != "charly-web" || hops[1] != "charly-web_db" {
		t.Fatalf("hops = %v, want [charly-web charly-web_db]", hops)
	}

	// The deploy-level member resolves too — dotted addressing covers BOTH
	// positions (the dead root-kind branch never chooses).
	node, _, err = ResolveDeployChain(roots, "web.zulu", ShellExecutor{})
	if err != nil {
		t.Fatalf("ResolveDeployChain(web.zulu): %v", err)
	}
	if node.Target != "vm" {
		t.Fatalf("deploy-level member target = %q, want vm", node.Target)
	}

	// A miss carries the available-members hint.
	_, _, err = ResolveDeployChain(roots, "web.nope", ShellExecutor{})
	if err == nil {
		t.Fatal("missing member must error")
	}
}

// executorHops flattens a NestedExecutor chain into its jump targets,
// outermost hop first.
func executorHops(chain spec.DeployExecutor) []string {
	var out []string
	for {
		nested, ok := chain.(*NestedExecutor)
		if !ok {
			// Reverse into outermost-first order (the walk collected leaf-up).
			for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
				out[i], out[j] = out[j], out[i]
			}
			return out
		}
		out = append(out, nested.Jump.Target)
		chain = nested.Parent
	}
}
