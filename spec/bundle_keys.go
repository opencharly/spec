package spec

import "sort"

// bundle_keys.go — deterministic bundle-map key ordering + the agent-provisioned venue predicate.
// Pure helpers over the bundle-tree spec types (BundleNode / UnifiedFile), relocated to the
// dedicated spec module (#55 2b Class A) so charly core + the loader-consuming plugins reach them
// without importing loaderkit. loaderkit re-exports them as forwarders for its own callers.

// SortedDeployKeys returns the deploy-map keys in deterministic (sorted) order.
func SortedDeployKeys(m map[string]BundleNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SortedMemberKeys returns the member keys of a node in deterministic order.
func SortedMemberKeys(members map[string]*BundleNode) []string {
	if len(members) == 0 {
		return nil
	}
	keys := make([]string, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// VenueIsAgentProvisioned reports whether the deploy node named venue (a child or member anywhere in
// the bundle tree) is flagged AgentProvisioned — the ONE genuinely bundle-tree-coupled predicate the
// check-run preflight needs to skip an agent-provisioned image's local-storage ensure.
func VenueIsAgentProvisioned(uf *UnifiedFile, venue string) bool {
	if uf == nil || venue == "" {
		return false
	}
	var walk func(n *BundleNode) bool
	walk = func(n *BundleNode) bool {
		if n == nil {
			return false
		}
		for k, child := range n.Children {
			if k == venue && child.AgentProvisioned {
				return true
			}
			if walk(child) {
				return true
			}
		}
		for k, member := range n.Members {
			if k == venue && member.AgentProvisioned {
				return true
			}
			if walk(member) {
				return true
			}
		}
		return false
	}
	for _, name := range SortedDeployKeys(uf.Bundle) {
		node := uf.Bundle[name]
		if walk(&node) {
			return true
		}
	}
	return false
}
