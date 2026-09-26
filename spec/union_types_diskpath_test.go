package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestKubevirtSource_DiskPathInImage pins the `disk_path_in_image` field on a
// container_disk source: it must survive the JSON wire so a schema-authored value
// reaches the plugin's CR render (the VMI `containerDisk.path`). This FAILS if the
// field is removed — the unmarshal silently drops it.
func TestKubevirtSource_DiskPathInImage(t *testing.T) {
	const wire = `{"kind":"container_disk","image":"localhost/charly-vm:latest","pull_policy":"Never","disk_path_in_image":"/disk.qcow2"}`
	var src KubevirtSource
	if err := json.Unmarshal([]byte(wire), &src); err != nil {
		t.Fatalf("unmarshal KubevirtSource: %v", err)
	}
	if src.DiskPathInImage != "/disk.qcow2" {
		t.Errorf("DiskPathInImage = %q, want /disk.qcow2 (field dropped?)", src.DiskPathInImage)
	}
	if src.Kind != "container_disk" || src.Image != "localhost/charly-vm:latest" || src.PullPolicy != "Never" {
		t.Errorf("sibling fields changed: %+v", src)
	}

	// The wire key is the authored snake_case name.
	out, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal KubevirtSource: %v", err)
	}
	if !strings.Contains(string(out), `"disk_path_in_image":"/disk.qcow2"`) {
		t.Errorf("marshaled wire = %s, want the disk_path_in_image key", out)
	}

	// An unauthored field stays empty (KubeVirt's default /disk scan).
	var empty KubevirtSource
	if err := json.Unmarshal([]byte(`{"kind":"container_disk","image":"x"}`), &empty); err != nil {
		t.Fatalf("unmarshal minimal: %v", err)
	}
	if empty.DiskPathInImage != "" {
		t.Errorf("unauthored DiskPathInImage = %q, want empty", empty.DiskPathInImage)
	}
}
