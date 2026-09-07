package spec

import "sort"

// deploy_keys.go — deterministic deploy-map key ordering + the agent-provisioned venue predicate.
// Pure helpers over the deploy-tree spec types (DeployNode / UnifiedFile), relocated to the
// dedicated spec module (#55 2b Class A) so charly core + the loader-consuming plugins reach them
// without importing loaderkit. loaderkit re-exports them as forwarders for its own callers.

// SortedDeployKeys returns the deploy-map keys in deterministic (sorted) order.
func SortedDeployKeys(m map[string]DeployNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// VenueIsAgentProvisioned reports whether the deploy node named venue (a child or member anywhere in
// the deploy tree) is flagged AgentProvisioned — the ONE genuinely deploy-tree-coupled predicate the
// check-run preflight needs to skip an agent-provisioned image's local-storage ensure.
func VenueIsAgentProvisioned(uf *UnifiedFile, venue string) bool {
	if uf == nil || venue == "" {
		return false
	}
	var walk func(n *DeployNode) bool
	walk = func(n *DeployNode) bool {
		if n == nil {
			return false
		}
		for i := range n.Member {
			m := &n.Member[i]
			if m.Name == venue && m.Node != nil && m.Node.AgentProvisioned {
				return true
			}
			if walk(m.Node) {
				return true
			}
		}
		return false
	}
	for _, name := range SortedDeployKeys(uf.Deploy) {
		node := uf.Deploy[name]
		if walk(&node) {
			return true
		}
	}
	return false
}
