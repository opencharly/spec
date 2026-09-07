package spec

import (
	"reflect"
	"strings"
	"testing"
)

// memberTree builds the canonical uniform member tree (Cutover C task 0): a pod
// deploy carrying BOTH member positions — a deploy-level sibling (brought up
// alongside) and an in-substrate member (deployed into the venue) — in a
// deliberately non-alphabetical authored order.
func memberTree() *Deploy {
	return &Deploy{
		Target: "pod",
		Member: []Member{
			{Name: "zulu", Position: PositionDeployLevel, Node: &Deploy{Target: "vm"}},
			{Name: "alpha", Position: PositionInSubstrate, Node: &Deploy{Target: "pod"}},
			{Name: "mike", Position: PositionDeployLevel, Node: &Deploy{Target: "local"}},
		},
	}
}

// TestMemberTreePreservesAuthoredOrder pins the ORDER half of the uniform member
// tree: the ONE ordered Member list keeps authored order, and both position
// filters preserve it (no map-iteration nondeterminism, no re-sorting).
func TestMemberTreePreservesAuthoredOrder(t *testing.T) {
	root := memberTree()
	all := root.Member
	if len(all) != 3 {
		t.Fatalf("member list length = %d, want 3", len(all))
	}
	for i, want := range []string{"zulu", "alpha", "mike"} {
		if all[i].Name != want {
			t.Fatalf("member[%d] = %q, want authored order %q", i, all[i].Name, want)
		}
	}
	alongside := root.DeployLevelMembers()
	if len(alongside) != 2 || alongside[0].Name != "zulu" || alongside[1].Name != "mike" {
		t.Fatalf("DeployLevelMembers = [%s, %s], want [zulu, mike] in authored order",
			alongside[0].Name, alongside[1].Name)
	}
	inSubstrate := root.InSubstrateMembers()
	if len(inSubstrate) != 1 || inSubstrate[0].Name != "alpha" {
		t.Fatalf("InSubstrateMembers = %v, want [alpha]", inSubstrate)
	}
}

// TestMemberPositionClassification pins the DERIVATION half: alongside-vs-
// deploy-into is derived from the entry's position (deploy-level vs in-substrate),
// never stored as a branch and never re-derived from the node's kind.
func TestMemberPositionClassification(t *testing.T) {
	root := memberTree()
	if !root.Member[0].Alongside() || root.Member[0].InSubstrate() {
		t.Fatalf("deploy-level member zulu classified wrong: alongside=%t in-substrate=%t",
			root.Member[0].Alongside(), root.Member[0].InSubstrate())
	}
	if !root.Member[1].InSubstrate() || root.Member[1].Alongside() {
		t.Fatalf("in-substrate member alpha classified wrong: alongside=%t in-substrate=%t",
			root.Member[1].Alongside(), root.Member[1].InSubstrate())
	}
	// A nil entry and a zero-position entry classify as neither (the fold stamps
	// the position; an un-stamped entry is neither class).
	var nilMember *Member
	if nilMember.Alongside() || nilMember.InSubstrate() {
		t.Fatal("nil member must classify as neither position")
	}
	unstamped := &Member{Name: "x", Node: &Deploy{}}
	if unstamped.Alongside() || unstamped.InSubstrate() {
		t.Fatal("un-stamped member must classify as neither position")
	}
	if m := root.MemberByName("alpha"); m == nil || m.Node.Target != "pod" {
		t.Fatalf("MemberByName(alpha) = %v, want the alpha entry", m)
	}
	if m := root.MemberByName("missing"); m != nil {
		t.Fatalf("MemberByName(missing) = %v, want nil", m)
	}
	if (*Deploy).MemberByName(nil, "any") != nil {
		t.Fatal("MemberByName on a nil node must be nil-safe")
	}
}

// TestDeployNodeHasNoDualMaps is the R5 gate: the dual Children/Members maps on
// the deploy node DIED in Cutover C task 0 — no dual representation survives.
func TestDeployNodeHasNoDualMaps(t *testing.T) {
	typ := reflect.TypeOf(Deploy{})
	for _, dead := range []string{"Children", "Members"} {
		if _, ok := typ.FieldByName(dead); ok {
			t.Fatalf("Deploy.%s must not exist — the dual member-tree maps died in Cutover C task 0 (one uniform ordered member tree)", dead)
		}
	}
	f, ok := typ.FieldByName("Member")
	if !ok || f.Type.Kind() != reflect.Slice || f.Type.Elem().Name() != "Member" {
		t.Fatalf("Deploy.Member must be the []Member uniform ordered member tree, got %v", f.Type)
	}
}

// TestStampDescentReachesUniformMembers pins that the descent stamper recurses
// the ONE member tree — both positions — not a per-class map pair.
func TestStampDescentReachesUniformMembers(t *testing.T) {
	traitsFor := func(word string) *DeployTraits {
		switch word {
		case "vm":
			return &DeployTraits{Venue: "ssh"}
		case "pod":
			return &DeployTraits{Venue: "container"}
		default:
			return nil
		}
	}
	root := memberTree()
	StampDescent(root, traitsFor)
	if root.Descent == nil || root.Descent.Transport != "container-exec" {
		t.Fatalf("root descent = %v, want container-exec", root.Descent)
	}
	if m := root.MemberByName("zulu"); m.Node.Descent == nil || m.Node.Descent.Transport != "ssh" {
		t.Fatalf("deploy-level member zulu descent = %v, want ssh", m.Node.Descent)
	}
	if m := root.MemberByName("alpha"); m.Node.Descent == nil || m.Node.Descent.Transport != "container-exec" {
		t.Fatalf("in-substrate member alpha descent = %v, want container-exec", m.Node.Descent)
	}
	// Idempotent re-stamp writes the same value.
	before := *root.MemberByName("zulu").Node.Descent
	StampDescent(root, traitsFor)
	after := *root.MemberByName("zulu").Node.Descent
	if before != after {
		t.Fatal("StampDescent must be idempotent")
	}
}

// TestValidateDeploymentMembers pins the dot-free key rule over the uniform
// member tree (both positions, every level).
func TestValidateDeploymentMembers(t *testing.T) {
	root := memberTree()
	if err := ValidateDeploymentMembers("", root); err != nil {
		t.Fatalf("valid member tree rejected: %v", err)
	}
	root.Member[2].Node.Member = []Member{
		{Name: "bad.key", Position: PositionInSubstrate, Node: &Deploy{Target: "pod"}},
	}
	err := ValidateDeploymentMembers("bed", root)
	if err == nil || !strings.Contains(err.Error(), "bad.key") {
		t.Fatalf("dotted member key not rejected: %v", err)
	}
}

// TestValidateDeployRequiresBoxMemberExemption pins the group exemption over the
// uniform list: a pod node carrying members (a venue, no own workload) needs no
// box; a pod LEAF without one is rejected.
func TestValidateDeployRequiresBoxMemberExemption(t *testing.T) {
	group := map[string]DeployNode{
		"venue": {Target: "pod", Member: []Member{
			{Name: "svc", Position: PositionInSubstrate, Node: &Deploy{Target: "pod", Image: "img"}},
		}},
	}
	if err := ValidateDeployRequiresBox(group); err != nil {
		t.Fatalf("member-bearing venue rejected: %v", err)
	}
	leaf := map[string]DeployNode{
		"lonely": {Target: "pod"},
	}
	if err := ValidateDeployRequiresBox(leaf); err == nil {
		t.Fatal("pod leaf without image must be rejected")
	}
}

// TestHasMembersUniform pins the member predicate on the ONE list. (The former IsGroup
// predicate died with the group kind — the dual-representation cutover: a targetless
// member-bearing deploy is no longer an authorable shape, the first member is the deploy's
// primary substrate node.)
func TestHasMembersUniform(t *testing.T) {
	root := memberTree()
	if !root.HasMembers() {
		t.Fatal("HasMembers = false, want true")
	}
	var empty *Deploy
	if empty.HasMembers() {
		t.Fatal("nil HasMembers must be false")
	}
	// The post-migrate spelling: a member-bearing deploy always carries its primary
	// substrate target — the former group fixture (targetless + members) is gone.
	g := Deploy{Target: "pod", Member: root.Member}
	if !g.HasMembers() {
		t.Fatal("targeted node with members must report HasMembers")
	}
}

// TestValidateDeploymentTreeUniform pins the tree entry point over the uniform
// member tree.
func TestValidateDeploymentTreeUniform(t *testing.T) {
	tree := map[string]DeployNode{
		"bed": {Target: "pod", Member: []Member{
			{Name: "alpha", Position: PositionInSubstrate, Node: &Deploy{Target: "pod", Image: "img"}},
		}},
	}
	if err := ValidateDeploymentTree(tree); err != nil {
		t.Fatalf("valid tree rejected: %v", err)
	}
	bad := map[string]DeployNode{
		"bed": {Target: "pod", Member: []Member{
			{Name: "dotted.key", Position: PositionDeployLevel, Node: &Deploy{Target: "vm"}},
		}},
	}
	if err := ValidateDeploymentTree(bad); err == nil {
		t.Fatal("dotted member key must be rejected through ValidateDeploymentTree")
	}
}
