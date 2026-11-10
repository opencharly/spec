package schema

// CLI reflection is a wire contract between command plugins, the Charly host,
// and MCP. It is CUE-owned like every other authored or transported shape.
#CLIArg: {
	prop: string & !=""
	name: string
	type: or(_cliArgTypeWord)
	help?: string
	enum?: [...string]
	default?: string
	has_default?: bool @go(HasDefault)
	required?: bool
	is_bool?: bool @go(IsBool)
	is_slice?: bool @go(IsSlice)
	is_map?: bool @go(IsMap)
	negated?: bool
	elem_type?: or(_elemTypeWord) @go(ElemType)
}

#CLILeaf: {
	path: string & !=""
	help?: string
	hidden?: bool
	positionals?: [...#CLIArg]
	flags?: [...#CLIArg]
}

#CLIModel: {
	name: string & !=""
	version: string
	leaves: [...#CLILeaf]
}

// --- vocabulary tables (S3) --------------------------------------------------
// Every closed set of words is a table keyed by word, `<x>Word` is derived from it,
// and the enum an author meets is `or(<x>Word)` — so the table and the enum cannot
// disagree and a new word is ONE row here. `@vocab` names the owner (§3.1): these
// sets are compiled into THIS contract module, so the owner is `spec`.
//
// Each table is named after its word when that name is unique in the schema, and
// `<definition><Field>` when it is not; the comment above it names its site.

// #CLIArg.type
_cliArgType: {
	"string": {}
	boolean: {}
	integer: {}
	"number": {}
	array: {}
	object: {}
} @vocab(spec)
_cliArgTypeWord: [for w, _ in _cliArgType {w}]
// #CLIArg.elem_type
_elemType: {
	"string": {}
	boolean: {}
	integer: {}
	"number": {}
	array: {}
	object: {}
} @vocab(spec)
_elemTypeWord: [for w, _ in _elemType {w}]
