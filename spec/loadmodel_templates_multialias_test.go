package spec

// loadmodel_templates_multialias_test.go — regression: fillNamespacedTemplates must
// fold EVERY alias path of a shared namespace repo, not only the first. The SAME
// repo (e.g. distro-arch) mounted at `charly.arch`, `charly.cachyos.arch`, and
// `charly.omarchy.arch` must expose its templates under ALL three qualified
// prefixes. A global pointer-visited guard folded only one (map-order dependent).

import (
	"encoding/json"
	"sort"
	"testing"
)

func TestProjectTemplates_MultiAliasSharedNamespace(t *testing.T) {
	// The shared repo: one namespace value carrying a vm template `arch`.
	arch := &UnifiedFile{
		PluginKinds: map[string]map[string]json.RawMessage{
			"vm": {"arch": json.RawMessage(`{"vm":{"source":{"kind":"iso"}}}`)},
		},
	}
	// The SAME *UnifiedFile mounted at three alias paths under a root.
	root := &UnifiedFile{
		Namespaces: map[string]*UnifiedFile{
			"arch": arch,
			"cachyos": {
				Namespaces: map[string]*UnifiedFile{"arch": arch},
			},
			"omarchy": {
				Namespaces: map[string]*UnifiedFile{"arch": arch},
			},
		},
	}
	vm := root.ProjectTemplates().ByKind("vm")
	for _, want := range []string{"arch.arch", "cachyos.arch.arch", "omarchy.arch.arch"} {
		if _, ok := vm[want]; !ok {
			t.Fatalf("template %q missing — a shared namespace repo mounted at multiple alias paths did not fold every alias; got keys %v", want, sortedRawKeys(vm))
		}
	}
}

func TestProjectTemplates_SelfCycleTerminates(t *testing.T) {
	// A namespace that (via the pointer-keyed cache) references itself must not
	// infinitely recurse — the ancestor stack terminates the cycle.
	self := &UnifiedFile{}
	self.Namespaces = map[string]*UnifiedFile{"self": self}
	// Should return without hanging or stack-overflowing.
	_ = self.ProjectTemplates()
}

// TestProjectTemplates_KindclusterFolds verifies the kindcluster substrate-template
// kind folds into the envelope like every other standalone-substrate kind (its
// PluginKinds[disc] bodies are copied + reachable via ByKind).
func TestProjectTemplates_KindclusterFolds(t *testing.T) {
	root := &UnifiedFile{
		PluginKinds: map[string]map[string]json.RawMessage{
			"kindcluster": {"lab": json.RawMessage(`{"kindcluster":{"box":"","engine":"podman"}}`)},
		},
	}
	kc := root.ProjectTemplates().ByKind("kindcluster")
	if _, ok := kc["lab"]; !ok {
		t.Fatalf("kindcluster template %q missing from the projection; got keys %v", "lab", sortedRawKeys(kc))
	}
}

func sortedRawKeys(m map[string]RawBody) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
