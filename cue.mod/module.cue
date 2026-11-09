// The spec module's CUE module. schema/ is ONE CUE package inside it, so a
// consumer (a plugin's own CUE module, or `cue` from this repo's root) can
// import the schema as a package instead of having its text concatenated.
//
// The language version is PINNED, not incidental: the schema's `close()` +
// embedded-definition idiom (schema/00_common.cue's #Op arms) is legal through
// v0.17 and rejected by v0.18.0's semantics ("field not allowed"), so v0.16.1 —
// the CUE library version this module pins in go.mod, which both the in-process
// params generator and every composing consumer evaluate under — is the version
// that keeps the package's VALUE identical to the concatenation's.
// Raising it is the §10.1 pattern rewrite (TODO 2.16), not a bump.
module: "github.com/opencharly/spec"

language: {
	version: "v0.16.1"
}
