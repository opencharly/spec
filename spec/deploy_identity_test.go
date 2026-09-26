package spec

import "testing"

// TestValidateDeploymentName_Identity pins the unified deploy-identity contract:
// an identity is a dot-joined path of non-empty dot-free SEGMENTS, optionally
// suffixed with `/<instance>`. A namespace-qualified key (`charly.check-docs`)
// is LEGAL (dots are the path join operator); a malformed identity (empty
// segment / empty instance) is not. A single SEGMENT still may not contain a dot
// or slash — those are the join/instance operators, not characters of a name.
func TestValidateDeploymentName_Identity(t *testing.T) {
	valid := []string{
		"check-docs",             // bare local
		"charly.check-docs",      // namespace-qualified
		"a.b.c.d",                // deep namespace/member path
		"versa/ecovoyage",        // instance axis
		"charly.versa/ecovoyage", // qualified + instance
	}
	for _, name := range valid {
		if err := ValidateDeploymentName(name, ""); err != nil {
			t.Errorf("ValidateDeploymentName(%q) = %v, want nil (a well-formed identity)", name, err)
		}
	}
	invalid := []string{
		"",           // empty
		".charly",    // leading dot
		"charly.",    // trailing dot
		"a..b",       // doubled dot
		"versa/",     // trailing instance separator
		"/ecovoyage", // missing name
	}
	for _, name := range invalid {
		if err := ValidateDeploymentName(name, ""); err == nil {
			t.Errorf("ValidateDeploymentName(%q) = nil, want error (malformed identity)", name)
		}
	}
}

// TestValidateDeploymentMembers_Segment pins that a member NAME is a single
// dot-free segment: a dot inside a segment would make the joined path ambiguous
// with member descent, so it is rejected; the join itself is the builder's job.
func TestValidateDeploymentMembers_Segment(t *testing.T) {
	ok := map[string]DeployNode{
		"bed": {Target: "pod", Member: []Member{
			{Name: "alpha", Position: PositionDeployLevel, Node: &Deploy{Target: "pod", Image: "img"}},
		}},
	}
	if err := ValidateDeploymentTree(ok); err != nil {
		t.Fatalf("valid member tree rejected: %v", err)
	}
	bad := map[string]DeployNode{
		"bed": {Target: "pod", Member: []Member{
			{Name: "dotted.key", Position: PositionDeployLevel, Node: &Deploy{Target: "vm"}},
		}},
	}
	if err := ValidateDeploymentTree(bad); err == nil {
		t.Fatal("a dotted member segment must be rejected through ValidateDeploymentTree")
	}
}
