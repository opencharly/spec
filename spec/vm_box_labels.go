// vm_box_labels.go — the VM-box metadata label contract (cutover plan task 2: the
// generic VM box). spec.VmBoxMetadata rides ONE OCI label — spec.LabelVmBox
// (ai.opencharly.vm.box) — whole-struct-marshaled as JSON, the VM analog of the
// per-field ai.opencharly.* label family the pod side names via deploykit's
// CapabilityLabelMap. Every exported field of VmBoxMetadata MUST have a VmBoxLabelMap
// entry: the map is the struct ↔ label sync table proving the WHOLE contract rides the
// one label, so a future source-less VM deploy (`charly fleet from-box vm:<ref>` /
// VmCapabilitiesFromLabels) can reconstruct every field from a pushed box image.
// Maintained alongside #VmBoxMetadata (CUE-sourced, schema/vm_box_metadata.cue) —
// adding a field to spec.VmBoxMetadata without an entry here trips
// CheckVmBoxLabelCompleteness and breaks the build.
package spec

import (
	"fmt"
	"reflect"
)

// VmBoxLabelMap names the OCI label each spec.VmBoxMetadata field rides. All fields
// share the single whole-struct label (LabelVmBox) — the values are uniform by
// design; the map's job is the completeness gate, not per-field routing.
var VmBoxLabelMap = map[string]string{
	// Guest identity.
	"Distro":      LabelVmBox,
	"Arch":        LabelVmBox,
	"Firmware":    LabelVmBox,
	"Init":        LabelVmBox,
	"Version":     LabelVmBox,
	"Description": LabelVmBox,

	// Adopted account + charly install strategy.
	"BaseUser":      LabelVmBox,
	"SSHUser":       LabelVmBox,
	"CharlyInstall": LabelVmBox,

	// Disk provenance + the baked check plan.
	"Source": LabelVmBox,
	"Plan":   LabelVmBox,
}

// CheckVmBoxLabelCompleteness returns an error listing any spec.VmBoxMetadata exported
// field that lacks a VmBoxLabelMap entry. Called from TestVmBoxLabelCompleteness to
// fail the build when a field is added without a label mapping.
func CheckVmBoxLabelCompleteness() error {
	rt := reflect.TypeFor[VmBoxMetadata]()
	var missing []string
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		if _, ok := VmBoxLabelMap[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("spec.VmBoxMetadata fields without VmBoxLabelMap entry: %v", missing)
	}
	return nil
}
