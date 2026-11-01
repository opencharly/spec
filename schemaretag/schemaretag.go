// Package schemaretag is THE single struct-tag normalization contract for the
// schema→Go pipeline (R3).
//
// `cue exp gengotypes` emits json tags only, but every committed
// `cue_types_gen.go` in this org carries yaml tags too — so the generation
// pipeline has a THIRD step after concatenation (`schemaconcat`) and generation:
// normalize the struct tags, doubling every json tag with a yaml tag of the same
// name and giving a bare yaml key `,omitempty`.
//
// That step used to live only inside `internal/schemagen`, which no other module
// may import (Go's internal rule) — which is why a charly-native regeneration
// verb could not reuse it, and why the recipe had to be published as
// checkout-relative shell. It lives here now: `internal/schemagen`'s retag mode
// and the `charly`-side generation verb both call THIS, so a generated file can
// never depend on which of them produced it.
//
// STANDALONE: depends only on the stdlib `regexp`, so both callers can import it
// without pulling in the packages they regenerate.
package schemaretag

import "regexp"

// reJSONOnlyTag matches a json tag that is the WHOLE tag literal — no yaml tag
// beside it. The leading backtick is what makes it "json-only": inside an already
// normalized `yaml:"x" json:"x"` the json part is preceded by a space, so a second
// pass cannot double it. That is what makes Normalize idempotent.
var reJSONOnlyTag = regexp.MustCompile("`json:\"([^\"]*)\"`")

// reBareYamlKey matches a yaml tag whose value is a bare key with no options. A
// comma means it already carries `,omitempty` (or another option), so the second
// pass cannot append a second one.
var reBareYamlKey = regexp.MustCompile(`yaml:"([a-zA-Z][^",]*)"`)

// Normalize rewrites a gengotypes-generated Go file's struct tags: every json-only
// tag gains a yaml tag of the same name, and every bare yaml key gains
// `,omitempty`. It is idempotent, and it touches nothing but those two tag forms —
// so a caller can normalize a generated file repeatedly without churn.
func Normalize(src []byte) []byte {
	out := reJSONOnlyTag.ReplaceAll(src, []byte("`yaml:\"${1}\" json:\"${1}\"`"))
	return reBareYamlKey.ReplaceAll(out, []byte(`yaml:"${1},omitempty"`))
}
