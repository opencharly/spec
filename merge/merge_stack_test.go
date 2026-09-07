package merge

import (
	"encoding/json"
	"testing"

	"github.com/opencharly/spec/spec"
)

// merge_stack_test.go — the layered config stack merge (system → user →
// project, later wins). Exercises MergeConfigStack: per-node later-wins,
// nil-layer skipping, per-layer discover anchoring, and version later-wins.

func candyNode(from string) json.RawMessage {
	return json.RawMessage(`{"from":"` + from + `"}`)
}

// TestMergeConfigStack_LaterWinsPerNode — a candy node declared in system is
// overridden by user, overridden by project; a node only in system survives.
func TestMergeConfigStack_LaterWinsPerNode(t *testing.T) {
	system := &spec.UnifiedFile{
		Candy: map[string]json.RawMessage{
			"alpha":   candyNode("system"),
			"sysonly": candyNode("system"),
		},
	}
	user := &spec.UnifiedFile{
		Candy: map[string]json.RawMessage{
			"alpha": candyNode("user"),
		},
	}
	project := &spec.UnifiedFile{
		Candy: map[string]json.RawMessage{
			"alpha": candyNode("project"),
		},
	}

	merged := MergeConfigStack(system, user, project, "/etc/charly", "/home/u/.config/charly", "/srv/proj")

	if got := string(merged.Candy["alpha"]); got != string(candyNode("project")) {
		t.Errorf("alpha: project should win, got %s", got)
	}
	if got := string(merged.Candy["sysonly"]); got != string(candyNode("system")) {
		t.Errorf("sysonly: system-only node should survive, got %s", got)
	}

	// The caller's layer files must not be mutated (each layer is copied).
	if got := string(system.Candy["alpha"]); got != string(candyNode("system")) {
		t.Errorf("system layer mutated: alpha = %s", got)
	}
	if got := string(user.Candy["alpha"]); got != string(candyNode("user")) {
		t.Errorf("user layer mutated: alpha = %s", got)
	}
	if got := string(project.Candy["alpha"]); got != string(candyNode("project")) {
		t.Errorf("project layer mutated: alpha = %s", got)
	}
}

// TestMergeConfigStack_NilLayersSkipped — system+user nil → project alone.
func TestMergeConfigStack_NilLayersSkipped(t *testing.T) {
	project := &spec.UnifiedFile{
		Version: "2026.3.0",
		Candy:   map[string]json.RawMessage{"alpha": candyNode("project")},
	}

	merged := MergeConfigStack(nil, nil, project, "/etc/charly", "/home/u/.config/charly", "/srv/proj")

	if merged.Version != "2026.3.0" {
		t.Errorf("version: got %q", merged.Version)
	}
	if got := string(merged.Candy["alpha"]); got != string(candyNode("project")) {
		t.Errorf("alpha: got %s", got)
	}
	if len(merged.Candy) != 1 {
		t.Errorf("candy map should carry only the project node, got %d entries", len(merged.Candy))
	}
}

// TestMergeConfigStack_DiscoverAnchoring — system's relative discover path is
// anchored to systemDir, user's to userDir; each stays anchored to its own
// layer's directory (not the project dir).
func TestMergeConfigStack_DiscoverAnchoring(t *testing.T) {
	system := &spec.UnifiedFile{
		Discover: spec.DiscoverConfig{{Path: "candy", Recursive: true}},
	}
	user := &spec.UnifiedFile{
		Discover: spec.DiscoverConfig{{Path: "candy", Recursive: true}},
	}
	project := &spec.UnifiedFile{
		Discover: spec.DiscoverConfig{{Path: "candy", Recursive: true}},
	}

	merged := MergeConfigStack(system, user, project, "/etc/charly", "/home/u/.config/charly", "/srv/proj")

	if len(merged.Discover) != 3 {
		t.Fatalf("discover should concatenate all three layers, got %d entries", len(merged.Discover))
	}
	paths := map[string]bool{}
	for _, s := range merged.Discover {
		paths[s.Path] = true
	}
	for _, want := range []string{"/etc/charly/candy", "/home/u/.config/charly/candy", "/srv/proj/candy"} {
		if !paths[want] {
			t.Errorf("discover missing anchored path %q (got %v)", want, paths)
		}
	}
}

// TestMergeConfigStack_VersionLaterWins — project's version wins; a project
// without a version falls back to the user's.
func TestMergeConfigStack_VersionLaterWins(t *testing.T) {
	system := &spec.UnifiedFile{Version: "2026.1.0"}
	user := &spec.UnifiedFile{Version: "2026.2.0"}

	// Project declares a version → it wins.
	project := &spec.UnifiedFile{Version: "2026.3.0"}
	merged := MergeConfigStack(system, user, project, "/etc/charly", "/home/u/.config/charly", "/srv/proj")
	if merged.Version != "2026.3.0" {
		t.Errorf("project version should win, got %q", merged.Version)
	}

	// Project declares no version → falls back to the user's.
	projectNoVer := &spec.UnifiedFile{}
	merged = MergeConfigStack(system, user, projectNoVer, "/etc/charly", "/home/u/.config/charly", "/srv/proj")
	if merged.Version != "2026.2.0" {
		t.Errorf("user version should win when project has none, got %q", merged.Version)
	}

	// Neither project nor user declares a version → system's survives.
	merged = MergeConfigStack(system, &spec.UnifiedFile{}, &spec.UnifiedFile{}, "/etc/charly", "/home/u/.config/charly", "/srv/proj")
	if merged.Version != "2026.1.0" {
		t.Errorf("system version should survive when later layers have none, got %q", merged.Version)
	}
}
