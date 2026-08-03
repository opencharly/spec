package spec

import "encoding/json"

// bundle_config.go — SPIKE: BundleConfig relocated from sdk/deploykit/deploy_state.go
// (#55 value-type relocation spike, cluster 2). Every field already resolved to a
// spec.* type (Provides=*spec.ProvidesConfig, Bundle=map[string]spec.BundleNode(=Deploy),
// Sidecar=map[string]json.RawMessage) so the type carries zero deploykit-only content.
// deploykit.BundleConfig becomes a type alias onto this type. Only the two PURE
// methods (Lookup/LookupKey, plain map access) moved with the type — the three
// methods that reach sdk/kit (DeployedContainerNames/OccupiedHostPorts/
// GlobalEnvForImage) stay in deploykit as free functions taking *spec.BundleConfig
// (spec can never import sdk/kit — the method-set cycle the spike flagged).

// BundleConfig represents per-machine deployment overrides (~/.config/charly/charly.yml).
// Only runtime/deployment fields are supported — build-time fields are structurally excluded.
//
// Schema v4: the top-level map key is `deployment:` (singular, flat). The
// legacy `images:` / `deployments.images.*` nesting is gone — all target
// kinds (host / vm / pod / k8s) live under the single `deployment:` map.
type BundleConfig struct {
	Provides *ProvidesConfig       `yaml:"provides,omitempty" json:"provides,omitempty"`
	Bundle   map[string]BundleNode `yaml:"deploy" json:"deploy"`
	// Sidecar carries the project's sidecar-template library as OPAQUE bodies
	// (the raw PluginKinds["sidecar"] map). candy/plugin-sidecar's OpResolve merges
	// these UNDER each deploy node's own overrides; the kernel reads no fields
	// (the sidecar de-type, Cutover D).
	Sidecar map[string]json.RawMessage `yaml:"sidecar,omitempty" json:"sidecar,omitempty"`
}

// Lookup returns the BundleNode for (deployName, instance), or
// (zero, false) when the entry is absent. Safe to call on a nil
// *BundleConfig — lets callers chain
// `loadDeployConfigForRead(...).Lookup(deployName, instance)` without a
// separate nil check. deployName is the charly.yml key base the caller is
// operating on (typically c.Image), NOT the baked image short-name — for a
// kind:check bed or Pattern-B deploy the two differ. Pass the deploy key, never
// a value derived from an image label (see MergeDeployOntoMetadata).
func (dc *BundleConfig) Lookup(deployName, instance string) (BundleNode, bool) {
	if dc == nil {
		return BundleNode{}, false
	}
	entry, ok := dc.Bundle[DeployKey(deployName, instance)]
	return entry, ok
}

// LookupKey looks up a deploy entry by its full charly.yml key (e.g.
// "foo", "foo/instance", "vm:name"). Safe on nil receiver.
func (dc *BundleConfig) LookupKey(key string) (BundleNode, bool) {
	if dc == nil {
		return BundleNode{}, false
	}
	entry, ok := dc.Bundle[key]
	return entry, ok
}
