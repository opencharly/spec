package deploy

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/opencharly/spec/spec"
)

// deploy_ops.go — the pure deploy-tree / deploy-path / candy-stage / preempt-resolve /
// task-var value HELPERS, sliced out of the spec contract module's spec/spec catch-all
// (#55 CHECK-ENGINE cone Option A — the deploy cone). Every one carries NO mechanism
// dependency (stdlib + spec's own value types only), so they are spec-hosted contract helpers
// an import-clean charly file can reach without an sdk mechanism-kit import — the FUNCTION
// analogue of the value TYPES (spec.DeployNode, spec.CandyReader) that stay in spec/spec.
// sdk/deploykit keeps thin re-export aliases (deploy_ops_aliases.go) so its own callers +
// tests + the deploy candies compile unchanged; charly repoints to spec/deploy.X directly.

// --- deploy-path helpers ---

// ResolveNodePath resolves a dotted deployment path against a root map, returning the leaf node,
// its ancestor chain, and any lookup error.
func ResolveNodePath(roots map[string]spec.DeployNode, path string) (*spec.DeployNode, []*spec.DeployNode, error) {
	parts := SplitDottedPath(path)
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("empty or malformed deployment path %q", path)
	}
	rootName := parts[0]
	rootEntry, ok := roots[rootName]
	if !ok {
		return nil, nil, fmt.Errorf("no deployment named %q", rootName)
	}
	current := &rootEntry
	var ancestors []*spec.DeployNode
	for i := 1; i < len(parts); i++ {
		ancestors = append(ancestors, current)
		member := current.MemberByName(parts[i])
		if member == nil || member.Node == nil {
			prefix := strings.Join(parts[:i], ".")
			return nil, nil, fmt.Errorf("no member %q under %q", parts[i], prefix)
		}
		current = member.Node
	}
	return current, ancestors, nil
}

// SplitDottedPath splits a dotted deployment path into segments. An empty input or a path with
// any empty segment (leading/trailing/doubled dots) yields nil so callers can flag the error at
// their layer with the original offending path string.
func SplitDottedPath(path string) []string {
	if path == "" {
		return nil
	}
	out := strings.Split(path, ".")
	if slices.Contains(out, "") {
		return nil
	}
	return out
}

// PathLeaf returns the last segment of a dotted deployment path.
// "foo.bar.baz" -> "baz"; "foo" -> "foo"; "" -> "". Unlike SplitDottedPath, a malformed path
// (leading/trailing/doubled dots) still yields its raw trailing segment rather than nil —
// callers that only care about the LEAF (e.g. a "host"/"local" literal-name check) want the
// tolerant form.
func PathLeaf(path string) string {
	if idx := strings.LastIndexByte(path, '.'); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// ClassifyNodeTarget picks the target discriminator for a node. Uses node.Target when non-empty
// (canonical pod|vm|kubernetes|local|android, set from the node-form kind by the loader's
// deployTargetForDisc). For ref-based deploys with no charly.yml entry, the deploy name itself is
// the hint: a literal `host`/`local` LEAF → local target; anything else → pod. A pure function of
// node+path with no LoadUnified/executor dependency.
func ClassifyNodeTarget(node *spec.DeployNode, path string) string {
	if node != nil && node.Target != "" {
		return node.Target
	}
	if leaf := PathLeaf(path); leaf == "host" || leaf == "local" {
		return "local"
	}
	return "pod"
}

// nodeDescentVenue reads a node's stamped descent VENUE trait (P9) nil-safely — the pure-data
// half of the former core-only nodeTraits/deployTraitDescent pair. A node with no stamped
// descent yields "" (the external-in-place default). This unifies the former deploykit
// deployNodeVenue(*DeployNode) + nodeVenue(DeployNode value) helpers into one (R3); a node
// sourced from LoadUnified/materialize or the resolved-project envelope is always stamped before
// any consult site sees it, so this needs no registry fallback.
func nodeDescentVenue(n *spec.DeployNode) string {
	if n != nil && n.Descent != nil {
		return n.Descent.Venue
	}
	return ""
}

// The uniform ordered member tree (Cutover C task 0) needs no sorted-key helper:
// the ONE ordered Member list is already deterministic in authored order.
// HostRooted reports whether node's stamped descent trait is host-rooted — the substrate's own
// ROOT executor runs directly on the host (a local/SSH-shell venue, not a container/VM venue).
// Reads the wire-stamped node.Descent directly (every node a LoadUnified'd project produces is
// Descent-stamped by StampDescent), so it needs no registry access. Promoted from sdk/deploykit's
// former private host-rooted predicate (#55 U4) so DeployNestedLocalChildren + the bed-session
// apply path (PersistBedDeployOverrides) share ONE predicate over the spec value type; deploykit
// callers repoint to spec/deploy.HostRooted directly.
func HostRooted(node *spec.DeployNode) bool {
	return node != nil && node.Descent != nil && node.Descent.HostRooted
}

// IsVmVenue reports whether node's stamped venue is the SSH-hop (vm) substrate. Mirrors
// HostRooted's shape (#55 W3 A4) — promoted so a plugin-side deploy-orchestration consumer
// (sdk/deploykit's BringUpMembers/TearDownMembers) and any future caller share ONE predicate over
// the wire-stamped node.Descent, instead of each re-deriving the venue check independently.
func IsVmVenue(node *spec.DeployNode) bool {
	return node != nil && node.Descent != nil && node.Descent.Venue == "ssh"
}

// IsContainerVenue reports whether node's stamped venue is the container-exec (pod) substrate.
// Mirrors HostRooted's shape (#55 W3 A4) — see IsVmVenue.
func IsContainerVenue(node *spec.DeployNode) bool {
	return node != nil && node.Descent != nil && node.Descent.Venue == "container"
}

// ExternalInPlaceVenue reports whether node's stamped venue is an EXTERNAL deploy substrate that
// applies its workload IN PLACE — local-like: no container image to build, no `charly
// config`/`charly start`, teardown via `charly deploy del` (replay the recorded reverse ops).
// local/android/kubernetes/exampledeploy are in-place (parent/none venues); pod is the one externalized
// substrate that is NOT in-place (excluded by requiring venue != container implicitly, since
// parent/none never equals container). Mirrors HostRooted's shape (#55 W3 B2-full) — the
// plugin-reachable equivalent of the former core-private bedExternalInPlace(target string), which
// queried isExternalDeploySubstrate against the live provider registry: every node this predicate
// sees comes from an already-loaded, Descent-stamped project, so the registry round-trip was
// redundant with data already on the wire (the SAME finding candy/plugin-fleet's
// externalInPlaceFromDescent already proved for a bed's sibling MEMBERS — this promotes that one
// shared predicate for the bed ROOT too, R3).
func ExternalInPlaceVenue(node *spec.DeployNode) bool {
	if node == nil || node.Descent == nil {
		return false
	}
	v := node.Descent.Venue
	return v == "parent" || v == "none"
}

// DeployNestedLocalChildren deploys a parent venue's IN-SUBSTRATE target:local members via the
// dotted-path dispatch — each host-rooted (local/SSH-shell) member applies its candies in place.
// Promoted from sdk/deploykit (#55 U4), now that HostRooted is a spec predicate; deploykit keeps a
// re-export forwarder so its charly callers compile unchanged.
//
// plugin-deploy-vm's PostApply brings up nested target:pod members as in-guest quadlets, but it
// SKIPS target:local members — they carry no image, they apply candies in place. Without this
// loop a nested local member never deploys, and a deploy-scope check against it either fails or
// (worse) silently checks nothing.
//
// Both sites that own a VM venue call it: the isVM bed ROOT and bringUpMembers' VM-member branch.
// They differ only in how a member deploy is executed (the root wraps it in a recorded step(); a
// member shells out directly), so that is the injected apply func.
func DeployNestedLocalChildren(parent string, node *spec.DeployNode, apply func(memberKey, dotted string) error) error {
	if node == nil {
		return nil
	}
	for _, m := range node.InSubstrateMembers() { // the deploy-into position only
		if m.Node == nil || !HostRooted(m.Node) { // local (host-rooted shell venue) only
			continue // container/vm members handled in-guest by plugin-deploy-vm's PostApply
		}
		if err := apply(m.Name, parent+"."+m.Name); err != nil {
			return fmt.Errorf("deploy nested local member %s.%s: %w", parent, m.Name, err)
		}
	}
	return nil
}

// BedCheckLiveRefs returns the ordered `charly check live` targets for a bed: the substrate
// itself first, then each IN-SUBSTRATE member as a dotted path in authored order. A
// `target: android` member shares the parent pod's venue (Descent.Venue == "parent") and has no
// own image — its app-presence checks are baked into the parent ref, so it is skipped. Pure +
// unit-tested.
func BedCheckLiveRefs(name string, node *spec.DeployNode) []string {
	if node == nil {
		return []string{name}
	}
	refs := []string{name}
	for _, m := range node.InSubstrateMembers() {
		if m.Node != nil && nodeDescentVenue(m.Node) == "parent" { // android (parent venue)
			continue
		}
		refs = append(refs, name+"."+m.Name)
	}
	return refs
}

// DescriptionInfo returns the first non-empty line of a candy/box description, trimmed.
func DescriptionInfo(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return ""
	}
	if before, _, ok := strings.Cut(d, "\n"); ok {
		return strings.TrimSpace(before)
	}
	return d
}

// MergeDeployNode overlays src onto dst: every authored (yaml-tagged, non-zero) field of src
// wins, and the loader-DERIVED structural TREE fields (Target, the uniform ordered Member
// tree) merge explicitly (src non-zero wins). Pure (reflect over the spec.DeployNode value
// type).
func MergeDeployNode(dst, src spec.DeployNode) spec.DeployNode {
	dstV := reflect.ValueOf(&dst).Elem()
	srcV := reflect.ValueOf(src)
	t := dstV.Type()
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i)
		tag := ft.Tag.Get("yaml")
		// Skip derived fields (yaml:"-") and untagged fields (rare; not part of the persisted
		// schema, so not merge-relevant).
		if tag == "-" || tag == "" {
			continue
		}
		sv := srcV.Field(i)
		if sv.IsZero() {
			continue
		}
		dstV.Field(i).Set(sv)
	}
	// Target + the uniform ordered Member tree are loader-DERIVED yet are real TREE DATA that
	// must merge across project + per-host overlay: src non-zero wins, else dst passes through.
	if src.Target != "" {
		dst.Target = src.Target
	}
	if len(src.Member) > 0 {
		dst.Member = src.Member
	}
	return dst
}

// --- candy-stage helpers ---

// CandyMapKey returns the candy's map key: the full @github ref for a remote candy
// (RepoPath/SubPathPrefix+Name), else the bare Name. A ROOT-LEVEL remote candy (the
// candy de-submodule cutover — a standalone candy repo whose manifest lives at the
// repo root, SubPathPrefix "") keys as the repo path itself: appending the name would
// double it (github.com/org/layer-ripgrep/layer-ripgrep).
func CandyMapKey(layer spec.CandyReader) string {
	if layer.GetRemote() {
		if layer.GetSubPathPrefix() == "" {
			return layer.GetRepoPath()
		}
		return layer.GetRepoPath() + "/" + layer.GetSubPathPrefix() + layer.GetName()
	}
	return layer.GetName()
}

// CandyStageDirName returns the content-addressed staging dir name for a remote candy's copied
// tree (name.version).
func CandyStageDirName(layer spec.CandyReader) string {
	if layer.GetVersion() == "" {
		return layer.GetName() // defensive; remote candies are mandatorily versioned
	}
	return layer.GetName() + "." + layer.GetVersion()
}

// --- preempt-resolve helpers ---

// HolderAddrFor derives the resource-arbiter holder address for a deploy-tree node — servable off
// a plain map[string]spec.DeployNode (the shape both a freshly-loaded uf.Deploy and a resolved-project
// envelope's Deploy map carry).
func HolderAddrFor(name string, node spec.DeployNode) spec.HolderAddr {
	base, instance := spec.ParseDeployKey(name)
	target := node.Target
	if target == "" {
		target = "pod"
	}
	addr := spec.HolderAddr{Name: name, Target: target, Base: base, Instance: instance}
	if nodeDescentVenue(&node) == "ssh" { // vm (ssh venue)
		addr.Vm = node.From
		if addr.Vm == "" {
			addr.Vm = base
		}
	}
	return addr
}

// FindVMClaimant returns the first node claiming the given VM entity via requires_exclusive.
func FindVMClaimant(tree map[string]spec.DeployNode, vmEntity string) (string, spec.DeployNode, bool) {
	for name, node := range tree {
		if nodeDescentVenue(&node) == "ssh" && node.From == vmEntity && len(node.RequiredExclusive()) > 0 {
			return name, node, true
		}
	}
	return "", spec.DeployNode{}, false
}

// --- task-var helpers ---

// TaskAutoExports are the auto-exported variable names reserved for the generator; `vars:`
// entries may not shadow these, and every `${VAR}` reference resolves against
// (auto-exports ∪ candy.Vars).
var TaskAutoExports = map[string]bool{
	"USER":       true,
	"UID":        true,
	"GID":        true,
	"HOME":       true,
	"ARCH":       true,
	"BUILD_ARCH": true,
}

// TaskKnownNames returns the ${NAME} references that resolve cleanly for this candy:
// auto-exports ∪ candy.Vars keys.
func TaskKnownNames(vars map[string]string) map[string]bool {
	known := make(map[string]bool, len(TaskAutoExports)+len(vars))
	for k := range TaskAutoExports {
		known[k] = true
	}
	for k := range vars {
		known[k] = true
	}
	return known
}

// --- artifact helpers ---

// CandyArtifactRegisters returns the DISTINCT `register:` hints declared across every candy's
// artifact list — name-blind (it reads each artifact's own declaration, never a candy name).
func CandyArtifactRegisters(layers []spec.CandyReader) map[string]bool {
	out := map[string]bool{}
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		for _, a := range layer.Artifact() {
			if a.Register != "" {
				out[a.Register] = true
			}
		}
	}
	return out
}
