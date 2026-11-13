package spec

// deploy_addcandy.go — the ONE collection of "which `add_candy:` refs does this deploy contribute,
// and at what composition scope" (opencharly/spec#309).
//
// Three consumers need exactly this traversal and must agree on it: the host collects it to widen the
// candy scan for plugin LOADING (charly/plugin_loader.go's deployNodePluginContext) and for the
// pod-overlay build (charly/build_overlay.go's overlayCandies), and the externalized `check` command
// collects it to widen its resolved-project request (candy/plugin-check's addCandyRefsForEntity,
// opencharly/plugin-check#147). A plugin cannot import core, so the shared home is HERE — beside the
// member-tree accessors the traversal is built from (member_tree.go) and the scope constructors it
// tags with (resolve_opts.go) — rather than a third hand-written loop.

// ScopedAddCandyRefs returns this node's own `add_candy:` refs — each tagged with the composition
// scope the deploy's box gives them — followed by its IN-SUBSTRATE descendants'.
//
// The traversal boundary is the authored POSITION, not the kind: a member authored INSIDE the kind
// body is deployed into its parent's venue and is part of its parent's composition, so its refs
// belong to the same scope. A DEPLOY-LEVEL member (a sibling of the kind key, Member.Alongside) is
// its own composition, addressed by its own name, and is deliberately NOT visited — its refs belong
// to its own scope, never the parent's.
//
// `scope` is the caller's composition label, normally `BoxScope(<deploy name>)`: a deploy IS its box
// (a deploy's add_candy: refs are that box's composition), which is why a constant label here would
// be wrong — two independent deploys share no scope and their pins legitimately differ (charly#739).
func (d *Deploy) ScopedAddCandyRefs(scope string) []ExtraCandyRef {
	var out []ExtraCandyRef
	var visit func(n *Deploy)
	visit = func(n *Deploy) {
		if n == nil {
			return
		}
		out = append(out, ScopedExtraCandyRefs(scope, n.AddCandy...)...)
		for _, m := range n.InSubstrateMembers() {
			visit(m.Node)
		}
	}
	visit(d)
	return out
}
