package spec

// member_tree.go — the uniform ordered member tree (Cutover C task 0, the R3
// member-tree rebuild). ONE member list per deploy node replaces the dual
// Children (nested-inside venue) / Members (brought-up alongside) maps: every
// member child hangs in Deploy.Member exactly once, in authored order, and the
// entry's Position carries WHERE it hung in the authored document (a deploy-level
// sibling of the kind key vs an entity key inside the kind body).
// Alongside-vs-deploy-into is DERIVED from that position at every consult site —
// never stored as a tree branch, never re-derived from the node's kind (the dead
// root-kind branch, sdk/loaderkit BuildDeployNode).

// Member positions — the authored-tree position vocabulary (the fold stamps
// Position from the authored depth alone).
const (
	// PositionDeployLevel — a deploy-level sibling of the kind key: a MEMBER
	// brought up alongside its parent on the shared network (never inside its
	// venue).
	PositionDeployLevel = "deploy-level"
	// PositionInSubstrate — an entity key inside the kind body: a NESTED member
	// deployed into the parent's venue.
	PositionInSubstrate = "in-substrate"
)

// Alongside reports whether the member is deployed ALONGSIDE its parent
// (deploy-level position): brought up beside it on the shared network, never
// inside its venue. The derived class of the deploy-level position.
func (m *Member) Alongside() bool {
	return m != nil && m.Position == PositionDeployLevel
}

// InSubstrate reports whether the member is deployed INTO its parent's venue
// (in-substrate position). The derived class of the in-substrate position.
func (m *Member) InSubstrate() bool {
	return m != nil && m.Position == PositionInSubstrate
}

// MemberByName resolves a member entry by its tree key (nil when absent) — the
// lookup twin of the former Children/Members map indexes over the ONE ordered
// list.
func (d *Deploy) MemberByName(name string) *Member {
	if d == nil {
		return nil
	}
	for i := range d.Member {
		if d.Member[i].Name == name {
			return &d.Member[i]
		}
	}
	return nil
}

// DeployLevelMembers returns the member entries authored at the DEPLOY level
// (brought up alongside), in authored order.
func (d *Deploy) DeployLevelMembers() []*Member {
	return d.membersByPosition(true)
}

// InSubstrateMembers returns the member entries authored INSIDE the substrate
// body (deployed into the venue), in authored order.
func (d *Deploy) InSubstrateMembers() []*Member {
	return d.membersByPosition(false)
}

func (d *Deploy) membersByPosition(alongside bool) []*Member {
	if d == nil {
		return nil
	}
	var out []*Member
	for i := range d.Member {
		if d.Member[i].Alongside() == alongside {
			out = append(out, &d.Member[i])
		}
	}
	return out
}

// HasMembers reports whether this node carries any member children (the uniform
// successor of the former HasChildren).
func (d *Deploy) HasMembers() bool {
	return d != nil && len(d.Member) > 0
}
