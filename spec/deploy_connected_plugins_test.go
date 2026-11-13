package spec

// The CONNECTED PLUGIN SET contract (opencharly/charly#1131): what the connect seam hands a
// consumer, and the ONE input key a class:step reads it back under. The wire KEY NAMES are the
// contract, not the Go field names — a consumer in another repo decodes this JSON — so they are
// what this test pins. Without the change the type has no such field and this does not compile;
// with a renamed wire key it goes RED.

import (
	"encoding/json"
	"testing"
)

func TestDeployPluginsConnectReplyCarriesTheConnectedSet(t *testing.T) {
	in := DeployPluginsConnectReply{
		Dir: "/project",
		Plugins: []DeployConnectedPlugin{
			{
				Key:      "plugin-deploy-firecracker",
				Leaf:     "plugin-deploy-firecracker",
				ViewPath: "/home/op/.cache/charly/plugin-build/views/plugin-deploy-firecracker-4acc8316daba136b",
				SrcDir:   "/project/candy/plugin-deploy-firecracker",
			},
			{
				// A source-less baked plugin: src_dir is absent, and stays absent on the wire.
				Key:      "plugin-mcp",
				Leaf:     "plugin-mcp",
				ViewPath: "/usr/lib/charly/plugins/plugin-mcp",
			},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	plugins, ok := wire["plugins"].([]any)
	if !ok || len(plugins) != 2 {
		t.Fatalf("wire reply does not carry a 2-entry \"plugins\" list: %s", raw)
	}
	first, _ := plugins[0].(map[string]any)
	for _, key := range []string{"key", "leaf", "view_path", "src_dir"} {
		if _, ok := first[key]; !ok {
			t.Errorf("connected-plugin entry is missing the wire key %q: %s", key, raw)
		}
	}
	second, _ := plugins[1].(map[string]any)
	if _, ok := second["src_dir"]; ok {
		t.Errorf("a source-less plugin must not emit src_dir: %s", raw)
	}

	var back DeployPluginsConnectReply
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(back.Plugins) != 2 || back.Plugins[0].ViewPath != in.Plugins[0].ViewPath || back.Plugins[0].Leaf != "plugin-deploy-firecracker" {
		t.Fatalf("round-trip lost the connected set: %+v", back.Plugins)
	}
}

// The input key is ONE spelling for the host that injects it and the plugin that reads it.
func TestConnectedPluginsInputKey(t *testing.T) {
	if ConnectedPluginsInputKey != "plugin_paths" {
		t.Fatalf("ConnectedPluginsInputKey moved off the published wire key: %q", ConnectedPluginsInputKey)
	}
}
