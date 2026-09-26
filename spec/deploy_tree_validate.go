package spec

import (
	"fmt"
	"strings"
)

// deploy_tree_validate.go — pure, kind-blind structural validation over an
// already-merged deployments tree (map[string]DeployNode), relocated from
// charly/unified.go (FLOOR-SLIM K1-proper mechanical batch). Every function
// here operates ONLY on DeployNode — no registry, no *UnifiedFile, no host
// I/O — so it belongs beside the wire type it validates (the same D-clause
// precedent as node_helpers.go's ClassifyDoc). The loader's validation chain calls
// ValidateDeploymentTree(merged.Deploy) directly; deploy_members.go calls
// ValidateDeploymentName for a folded peer member's key.

// ValidateDeploymentTree enforces structural invariants on the deployments tree
// that can't be expressed in the YAML struct tags:
//
//   - Every key is a well-formed deploy IDENTITY: a dot-joined path of
//     non-empty segments, optionally suffixed with `/<instance>`
//     (ValidateDeploymentName). Dots ARE legal — they separate the
//     namespace/member path (that is what a qualified key means).
//   - Every explicit pod deploy must declare `box:` (ValidateDeployRequiresBox).
//
// Errors include the offending path so the user sees exactly which entry needs
// to be fixed.
func ValidateDeploymentTree(deploy map[string]DeployNode) error {
	if deploy == nil {
		return nil
	}
	for name, node := range deploy {
		if err := ValidateDeploymentName(name, ""); err != nil {
			return err
		}
		if err := ValidateDeploymentMembers(name, &node); err != nil {
			return err
		}
	}
	if err := ValidateDeployRequiresBox(deploy); err != nil {
		return err
	}
	return nil
}

// ValidateDeployRequiresBox enforces the 2026-05-12 schema rule:
// every `target: pod` deploy entry MUST declare its `box:` field.
// Pre-cutover the check runner silently fell back to inspecting the
// running container's image ref via `containerImageRef`, which read
// stale OCI labels off volume-pinned containers and dropped any
// probes added after the seed image. The hard-required field forces
// operator intent to be explicit; the check runner now resolves the
// ref ONLY from this field.
//
// Scope: target: pod (or empty — pod is the default). target: vm
// uses `vm:`, target: local is candy-driven, target: kubernetes
// CLUSTER definitions live in the `kubernetes:` section (not deploy:).
//
// Remediation: `charly migrate` (idempotent) walks every
// affected deploy and injects the field, inferring the value from
// the deploy key (`<base>` for `<base>/<instance>` keys; the key
// itself otherwise).
func ValidateDeployRequiresBox(deploy map[string]DeployNode) error {
	for name, node := range deploy {
		// An iterate: benchmark (the former kind:score) composes its scored
		// subject via plan `include:` steps + the iterate.sandbox, NOT a single
		// `box:`. It is exempt from the pod-target box requirement; its own
		// invariants are checked by validateCheckBeds (iterate block validation).
		if node.Iterate != nil {
			continue
		}
		// An agent-provisioned member carries NO box: by design — the AI builds
		// its image at run time (the iterate-benchmark contract). Exempt it from
		// the pod-target box requirement.
		if node.AgentProvisioned {
			continue
		}
		target := node.Target
		// Only an explicit pod-target (a `pod` node, or a `deploy` that inferred pod
		// from a box) is box-required. An EMPTY target is a group / per-host overlay
		// entry (no workload), never a pod-leaf — in node-form a real pod always
		// carries its box (the target is inferred FROM the box), so an empty target
		// can only be a group, which needs no box.
		if target != "pod" {
			continue
		}
		if node.Image == "" {
			// A deploy GROUP / venue (no own workload) carries members but no
			// box of its own — its member nodes each declare their box and are
			// validated as folded top-level entries. Only a LEAF pod-workload
			// (no members) must declare box.
			if len(node.Member) > 0 {
				continue
			}
			return fmt.Errorf(
				"deploy entry %q lacks required `box:` field — a pod-target deploy must declare `box:` explicitly (the check runner reads the operator's declared intent, not the running container's stale label)",
				name,
			)
		}
	}
	return nil
}

// ValidateDeploymentMembers recurses the SEGMENT rule over node's uniform ordered
// member tree (Cutover C task 0). A member NAME is a single identity SEGMENT: the
// dot is the JOIN operator owned by the path builder (`parent.member`), so it must
// not appear INSIDE a segment — that keeps the joined identity unambiguous with
// member-path descent. Every member's key is validated at every level.
func ValidateDeploymentMembers(path string, node *DeployNode) error {
	if node == nil || len(node.Member) == 0 {
		return nil
	}
	for i := range node.Member {
		m := &node.Member[i]
		childPath := m.Name
		if path != "" {
			childPath = path + "." + m.Name
		}
		if err := validateIdentitySegment(m.Name, childPath); err != nil {
			return err
		}
		if err := ValidateDeploymentMembers(childPath, m.Node); err != nil {
			return err
		}
	}
	return nil
}

// validateIdentitySegment rejects a single path SEGMENT (a member name) that is
// empty, carries a dot (the join operator belongs BETWEEN segments), or carries
// the `/` instance separator (which belongs only at the end of a full identity).
func validateIdentitySegment(seg, full string) error {
	if seg == "" {
		return fmt.Errorf("deploy identity %q has an empty path segment", full)
	}
	if strings.ContainsAny(seg, "./") {
		return fmt.Errorf("deploy identity segment %q (in %q) contains '.', the path join separator, or '/', the instance separator — a segment is a single dot-free name; the identity joins segments with '.' and appends `/<instance>`", seg, full)
	}
	return nil
}

// ValidateDeploymentName enforces that a deploy IDENTITY is well-formed: a
// dot-joined path of NON-EMPTY segments, optionally followed by `/<instance>`.
//
// Dots are LEGAL and MEANINGFUL — a dotted key is a NAMESPACE-QUALIFIED (or nested)
// deploy identity (`charly.check-docs`), the single string used unchanged as the
// tree key, the per-host overlay key, the CLI address, and the lookup key. The
// former rule that forbade dots existed only because the overlay used to be keyed
// by a lossy `vm:<dashed>` projection and dotted-path addressing was mistaken for a
// key constraint; with the identity unified on the dotted form, a dot is no longer
// a defect. What is rejected is a MALFORMED identity: an empty segment
// (leading/trailing/doubled `.`) or an empty instance (a trailing `/`) — the same
// malformed-path rule SplitDottedPath applies.
func ValidateDeploymentName(name, parentPath string) error {
	full := name
	if parentPath != "" {
		full = parentPath + "." + name
	}
	path := full
	if before, instance, ok := strings.Cut(path, "/"); ok {
		if instance == "" {
			return fmt.Errorf("deploy identity %q has an empty instance segment — a `/` separates the optional instance (e.g. `versa/ecovoyage`)", full)
		}
		path = before
	}
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			return fmt.Errorf("deploy identity %q has an empty path segment — `.` joins the namespace/member path and must not lead, trail, or repeat", full)
		}
	}
	return nil
}
