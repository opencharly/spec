package merge

import (
	"encoding/json"

	"github.com/opencharly/spec/spec"
)

// merge_stack.go — the layered config stack merge (system → user → project),
// the §3.5 half of the systemd-units + /etc/charly config work. A thin wrapper
// over the existing MergeUnified (R3 — no new merge logic): the stack is three
// plain UnifiedFiles (the ONE charly.yml format, §1.1) and the merge is
// LATER-WINS, uniform over the full UnifiedFile surface.

// MergeConfigStack merges the layered config stack (system → user → project)
// into one spec.UnifiedFile, LATER files winning (project > user > system).
//
// Each layer is a plain UnifiedFile: the system layer (/etc/charly/charly.yml),
// the user layer (~/.config/charly/charly.yml), and the in-dir project layer
// (<project>/charly.yml). The merge reuses MergeUnified (dst-wins) with the
// LATER file as dst, so a node or field declared in a later layer overrides the
// earlier one, and earlier-only declarations survive as gaps the later layer
// fills.
//
// Each layer's Discover specs are pre-anchored to that layer's OWN directory
// (spec.AnchorScanSpecs) BEFORE merging, so MergeUnified's srcDir re-anchoring
// is a no-op (AnchorScanSpecs passes absolute paths through unchanged) and a
// system-layer discover root stays anchored to the system dir rather than to
// the project dir.
//
// Each layer is copied before merging — the caller's files are never mutated.
// Nil layers are skipped.
func MergeConfigStack(system, user, project *spec.UnifiedFile, systemDir, userDir, projectDir string) *spec.UnifiedFile {
	merged := &spec.UnifiedFile{}
	if system != nil {
		merged = cloneUnified(system)
		merged.Discover = spec.AnchorScanSpecs(merged.Discover, systemDir)
	}
	if user != nil {
		userCopy := cloneUnified(user)
		userCopy.Discover = spec.AnchorScanSpecs(userCopy.Discover, userDir)
		// user wins over system: user is dst.
		MergeUnified(userCopy, merged, userDir)
		merged = userCopy
	}
	if project != nil {
		projectCopy := cloneUnified(project)
		projectCopy.Discover = spec.AnchorScanSpecs(projectCopy.Discover, projectDir)
		// project wins over user: project is dst.
		MergeUnified(projectCopy, merged, projectDir)
		merged = projectCopy
	}
	return merged
}

// cloneUnified deep-copies the map/slice fields MergeUnified mutates on its dst
// (Discover, Box, Candy, Deploy, PluginKinds) so merging never mutates the
// caller's layer files. Value fields (Version/Repo), the Defaults struct, and
// the never-mutated pointer fields (Provides/Cache/Ledger/System) are carried
// by the struct copy.
func cloneUnified(uf *spec.UnifiedFile) *spec.UnifiedFile {
	if uf == nil {
		return nil
	}
	c := *uf
	c.Discover = append([]spec.ScanSpec(nil), uf.Discover...)
	c.Box = cloneRawMap(uf.Box)
	c.Candy = cloneRawMap(uf.Candy)
	c.Deploy = cloneDeployMap(uf.Deploy)
	c.PluginKinds = clonePluginKinds(uf.PluginKinds)
	return &c
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneDeployMap(m map[string]spec.DeployNode) map[string]spec.DeployNode {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]spec.DeployNode, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func clonePluginKinds(m map[string]map[string]json.RawMessage) map[string]map[string]json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]map[string]json.RawMessage, len(m))
	for kind, entities := range m {
		out[kind] = cloneRawMap(entities)
	}
	return out
}
