package spec

import (
	"encoding/json"
	"testing"
)

// plugin_kinds_config_test.go — coverage for the Config.PluginKinds threading: projectConfigCached
// threads every plugin kind's raw bodies (uf.PluginKinds) into Config.PluginKinds, at the root and in
// every imported namespace, so the build/deploy kit can hand a content kind its own nodes from the
// box's OWNING scope (ops.OpContent).

// TestConfig_PluginKindsThreading proves the raw bodies thread verbatim, per kind word, and that a
// namespace's projection carries the namespace's OWN nodes rather than the root's.
func TestConfig_PluginKindsThreading(t *testing.T) {
	rootBody := json.RawMessage(`{"family":"charly-infrastructure","owner":"postgresql"}`)
	nsBody := json.RawMessage(`{"family":"distros","owner":"arch-skills"}`)
	uf := &UnifiedFile{
		RootDir: "/tmp",
		PluginKinds: map[string]map[string]json.RawMessage{
			"skill": {"postgresql-skill": rootBody},
		},
		Namespaces: map[string]*UnifiedFile{
			"arch": {RootDir: "/tmp/arch", PluginKinds: map[string]map[string]json.RawMessage{
				"skill": {"arch-skill": nsBody},
			}},
		},
	}
	cfg := uf.ProjectConfig()
	if cfg == nil {
		t.Fatal("ProjectConfig() = nil")
	}
	if got := string(cfg.PluginKinds["skill"]["postgresql-skill"]); got != string(rootBody) {
		t.Fatalf("root PluginKinds[skill] body = %q, want it threaded verbatim", got)
	}
	ns := cfg.Namespaces["arch"]
	if ns == nil {
		t.Fatal("namespace arch was not projected")
	}
	if got := string(ns.PluginKinds["skill"]["arch-skill"]); got != string(nsBody) {
		t.Fatalf("namespace PluginKinds[skill] body = %q, want the namespace's own node", got)
	}
	if _, leaked := ns.PluginKinds["skill"]["postgresql-skill"]; leaked {
		t.Fatal("the root's node leaked into the namespace's projection")
	}
}
