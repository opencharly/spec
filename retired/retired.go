// Package retired publishes the contract's canonical inventory of WITHDRAWN
// identifiers — the names the contract has retired — and the instruments that
// assert they stay withdrawn.
//
// WHY one shared list and not scattered literals: a withdrawn name that lives
// only inside the assertion that guards it is a name no other control can
// read. The next claim class — the same name re-added somewhere else in the
// org — is then missed the same way, by the same mechanism, because the only
// copy of the name is inside the guard that already passed. The list is the
// one place the names live, so every control reads the same names: this
// package's own guard, and the org's other controls.
//
// The list is DATA, published as JSON beside this file. It is a control's key
// set, not an authored configuration and not a wire shape: nothing loads it as
// a project, nothing carries it over the wire, and no schema-shaped struct is
// transcribed from it. It is also machine-readable by every reader — the Go
// guard below, and the org's non-Go controls — so one file serves them all
// with no projection and no regeneration step.
//
// A name is withdrawn ON A SURFACE, and the surfaces are withdrawn
// independently: a Go field can be renamed while its yaml key stays, and a
// yaml key can be withdrawn while the Go field keeps its name. So the list
// records both, and each entry carries the rule that catches it.
package retired

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed identifiers.json
var registryJSON []byte

// Kind is the surface an identifier was withdrawn from.
type Kind string

const (
	// KindField is a Go struct field.
	KindField Kind = "field"
	// KindConfigKey is an authored configuration key — a yaml/json tag, a CUE label.
	KindConfigKey Kind = "config-key"
	// KindEnv is an environment variable.
	KindEnv Kind = "env"
	// KindFlag is a command-line flag.
	KindFlag Kind = "flag"
)

// Identifier is one withdrawn name.
//
// Type scopes the name to one Go type. A name withdrawn from ONE type — a
// selector that only ever lived on the runtime's engine config — is not
// withdrawn everywhere, and an unscoped rule for it would condemn the same
// word where it is still legal (`build` is a legal key on the box schema). An
// empty Type means the name is withdrawn from every type.
//
// The scope also decides the RULE: an unscoped name is matched as a token
// wherever it can appear, and a scoped name is matched only inside its scope.
// A scoped name is never matched as a bare token, because a short word such
// as `run` would then match inside `runtime`.
type Identifier struct {
	Kind       Kind   `json:"kind"`
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	ReplacedBy string `json:"replacedBy,omitempty"`
	Why        string `json:"why"`
}

// Scoped reports whether the identifier is confined to one Go type.
func (i Identifier) Scoped() bool { return i.Type != "" }

// String renders the identifier the way it is spoken about: the type it
// lived on, then the name.
func (i Identifier) String() string {
	if i.Type != "" {
		return i.Type + "." + i.Name
	}
	return i.Name
}

// registry is the decoded identifiers file.
type registry struct {
	Identifiers []Identifier `json:"identifiers"`
}

// Identifiers is the contract's canonical inventory of withdrawn names.
var Identifiers = mustLoad()

// mustLoad decodes the embedded registry. A malformed registry is a defect in
// a committed file, not a runtime condition to recover from, so it stops the
// process here rather than at the first read.
func mustLoad() []Identifier {
	var reg registry
	if err := json.Unmarshal(registryJSON, &reg); err != nil {
		panic(fmt.Sprintf("retired: decoding the identifiers registry: %v", err))
	}
	return reg.Identifiers
}
