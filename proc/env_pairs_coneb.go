package proc

// env_pairs_coneb.go — the env-map → sorted KEY=VALUE pairs helper, sliced out of the spec
// contract module's spec/spec catch-all (#55 CHECK-ENGINE cone Option A — the process/launch
// cone) and folded into spec/proc alongside the process-launch fabric (proc/launch.go's former
// duplicate body is deleted — R3, ONE copy in this package). The body is verbatim from
// sdk/kit/env.go; sdk/kit re-exports it (var EnvMapToPairs = proc.EnvMapToPairs) so existing
// kit.EnvMapToPairs call sites (sdk/deploykit's deploy_file.go + read_labels.go, kit/box_metadata.go)
// are unchanged. spec/container's ExtractMetadata (box_metadata_coneb.go) calls proc.EnvMapToPairs
// directly — the canonical home.

import "sort"

// EnvMapToPairs converts the deploy schema's env map into sorted KEY=VALUE
// pairs (the OCI-label wire + env-resolution chain form).
func EnvMapToPairs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}
