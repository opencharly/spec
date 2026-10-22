package spec

import (
	"encoding/json"
	"testing"
)

// TestApplyInitExtendsSharesTheOneTemplate is the R2 gate: a variant entry
// inherits the base's service_template (so there is ONE authored template), keeps
// its OWN fields, clears Extends, and carries the inherited template in Raw —
// which is what the service-render leg actually decodes.
func TestApplyInitExtendsSharesTheOneTemplate(t *testing.T) {
	base := &ResolvedInit{
		Model:                "file_copy",
		CandyFiles:           []string{"*.service"},
		ManagementTool:       "systemctl",
		SystemEnableTemplate: "RUN systemctl enable {{.Units}}\n",
		ServiceSchema:        &InitServiceSchema{ServiceTemplate: "[Unit]\nDescription={{.Name}}\n"},
	}
	child := &ResolvedInit{
		Model:      "file_copy",
		Extends:    "systemd",
		Entrypoint: []string{"/sbin/init"},
	}
	inits := map[string]*ResolvedInit{"systemd": base, "systemd-container": child}
	ApplyInitExtends(inits)

	if child.Extends != "" {
		t.Errorf("Extends = %q after merge, want cleared", child.Extends)
	}
	if child.ServiceSchema == nil || child.ServiceSchema.ServiceTemplate != base.ServiceSchema.ServiceTemplate {
		t.Errorf("service_template not inherited: %+v", child.ServiceSchema)
	}
	if child.ManagementTool != "systemctl" || len(child.CandyFiles) != 1 {
		t.Errorf("base fields not inherited: tool=%q files=%v", child.ManagementTool, child.CandyFiles)
	}
	if len(child.Entrypoint) != 1 || child.Entrypoint[0] != "/sbin/init" {
		t.Errorf("the child's OWN field was overwritten: %v", child.Entrypoint)
	}
	if len(child.Raw) == 0 {
		t.Fatal("Raw was not rewritten to the merged envelope — the render leg would not see the inherited template")
	}
	var authored Init
	if err := json.Unmarshal(child.Raw, &authored); err != nil {
		t.Fatalf("merged Raw does not decode as an authored #Init: %v", err)
	}
	if authored.ServiceSchema == nil || authored.ServiceSchema.ServiceTemplate == "" {
		t.Errorf("merged Raw carries no service_template: %+v", authored.ServiceSchema)
	}
}

// TestApplyInitExtendsLeavesABrokenChainInert pins the documented failure mode: an
// unknown base or a cycle performs NO partial merge, so a typo cannot half-build
// an init entry from a fragment of a base.
func TestApplyInitExtendsLeavesABrokenChainInert(t *testing.T) {
	unknown := &ResolvedInit{Extends: "does-not-exist", Entrypoint: []string{"/sbin/init"}}
	cyclicA := &ResolvedInit{Extends: "b", Entrypoint: []string{"/a"}}
	cyclicB := &ResolvedInit{Extends: "a", Entrypoint: []string{"/b"}}
	inits := map[string]*ResolvedInit{
		"unknown": unknown,
		"a":       cyclicA,
		"b":       cyclicB,
	}
	ApplyInitExtends(inits)
	if unknown.Extends != "does-not-exist" || len(unknown.Entrypoint) != 1 {
		t.Errorf("unknown base must leave the child as authored: %+v", unknown)
	}
	if cyclicA.Extends != "b" || cyclicB.Extends != "a" {
		t.Errorf("a cycle must leave both entries as authored: a=%+v b=%+v", cyclicA, cyclicB)
	}
}
