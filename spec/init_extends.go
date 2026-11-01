package spec

import (
	"encoding/json"
	"reflect"
)

// init_extends.go — the init vocabulary's `extends:` merge.
//
// WHY IT EXISTS. Two entries can be genuinely the SAME init in different venues:
// systemd: is the machine/bootc init, systemd-container: is the container-venue
// init. They share one service_template verbatim and differ only in what the
// venue needs (entrypoint, system/user scope, masks, runtime flags). Without a
// merge the second entry is a copy-pasted YAML stanza — the exact duplication R2
// forbids. `extends:` names the base entry of the SAME vocabulary; the child's
// own non-empty fields win and the base fills the rest.
//
// WHY NOT A YAML ANCHOR. The loader re-parses each top-level node in ISOLATION
// when it builds the embedded/project defaults, so an alias in one entry cannot
// see an anchor defined in another ("unknown anchor" — measured). The merge is
// therefore a resolve-time step, not a document-level one.
//
// THE MERGE RULE. For every field except Raw and Extends: a child's zero value
// (empty string, false, nil/empty slice or map, nil pointer) INHERITS the base's
// value. There is deliberately no "explicitly empty" escape hatch — no init
// needs one, and inventing a sentinel for a field nobody needs is worse than the
// limitation. Raw is the child's opaque authored body; after the merge it is
// REWRITTEN to the merged envelope so the service-render leg (which decodes Raw,
// never the resolved fields) sees the inherited service_schema.
//
// FAILURE MODE. A base that does not exist, or a cycle, leaves the child exactly
// as authored (no partial merge). That is inert rather than silent-corrupt: the
// entry simply does not gain the base's fields, and a child that then lacks a
// service_template cannot render custom service entries. Both are authoring
// errors in a vocabulary's own charly.yml; the merge never fabricates fields.

// ApplyInitExtends resolves every `extends:` chain in an init vocabulary map, in
// place. Safe on a nil/empty map and idempotent (a resolved child carries no
// Extends).
func ApplyInitExtends(inits map[string]*ResolvedInit) {
	if len(inits) == 0 {
		return
	}
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(inits))
	var apply func(name string)
	apply = func(name string) {
		switch state[name] {
		case visiting, done:
			return
		}
		state[name] = visiting
		child := inits[name]
		if child == nil || child.Extends == "" {
			state[name] = done
			return
		}
		baseName := child.Extends
		base, ok := inits[baseName]
		// The WHOLE chain is checked before any merge: an unknown base anywhere in
		// it, or a cycle, leaves the child exactly as authored. Checking only the
		// immediate base lets a cycle merge one arm from the other, which is the
		// partial merge this rule exists to forbid.
		if !ok || base == nil || !initExtendsChainOK(inits, name) {
			state[name] = done
			return
		}
		apply(baseName)
		base = inits[baseName]
		if base == nil {
			state[name] = done
			return
		}
		mergeResolvedInit(child, base)
		child.Extends = ""
		// The render leg decodes Raw; rewrite it so the inherited service_schema
		// and templates are visible there too.
		merged := *child
		merged.Raw = nil
		if b, err := json.Marshal(merged); err == nil {
			child.Raw = b
		}
		state[name] = done
	}
	for name := range inits {
		apply(name)
	}
}

// initExtendsChainOK walks name's `extends:` chain and reports whether every
// link names an existing init and no init repeats (a cycle). Called BEFORE any
// merge so a broken chain is inert end to end.
func initExtendsChainOK(inits map[string]*ResolvedInit, name string) bool {
	seen := map[string]bool{name: true}
	cur := name
	for {
		entry := inits[cur]
		if entry == nil {
			return false
		}
		next := entry.Extends
		if next == "" {
			return true
		}
		if seen[next] {
			return false
		}
		seen[next] = true
		cur = next
	}
}

// mergeResolvedInit copies base's non-zero fields into dst where dst is zero.
// Reflectively, because #ResolvedInit is generated: an explicit field list would
// silently skip every field a future schema adds.
func mergeResolvedInit(dst, base *ResolvedInit) {
	dv := reflect.ValueOf(dst).Elem()
	bv := reflect.ValueOf(base).Elem()
	t := dv.Type()
	for i := 0; i < t.NumField(); i++ {
		switch t.Field(i).Name {
		case "Raw", "Extends":
			// Raw belongs to the child (rewritten by the caller); Extends is
			// consumed by this merge.
			continue
		}
		if !dv.Field(i).IsZero() {
			continue
		}
		dv.Field(i).Set(bv.Field(i))
	}
}
