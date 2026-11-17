package schema

// secret.cue — the secret delivery contract (§8), PORTED from DESIGN.cue, not re-designed.
//
// DESIGN.cue declares `#SecretUse` at :474-480 and `#AbsPath` at :77; this file is that declaration
// moved into the contract module so the types can be generated from it.
//
// WHY IT EXISTS AT ALL, and it is the whole point of D-SEC-1: a secret reaches its consumer as a FILE
// on the venue, never as text in the IR. DESIGN.md:2511 -- *"Secrets come only from the configured
// backend; they never enter the store, artifacts, labels, IR, state or output."* DESIGN.md:2983 adds
// the other half: *"there is no `get`: a value reaches a consumer as a `secret:` need, never as text."*
//
// So a secret `use:` entry says WHERE the value comes from and WHERE it lands -- never WHAT it is. The
// two paths are `#AbsPath`, which forbids `$` precisely so a path cannot smuggle a resolved value
// through variable expansion.
//
// A secret is a `use:`, not a `provide:`. `_capability.secret` carries `need`, `use` and NO `provide`
// field (DESIGN.cue:485), so no authored stack member can hand a secret to another: it comes from the
// configured backend and nowhere else.

// #AbsPath — a literal absolute path. DESIGN.cue:77.
//
// The `[^$]*` is load-bearing rather than cosmetic: it is how D-SCH-7 keeps a variable OUT of a path,
// so a path can never expand into a value. A schema that allowed `$` here would reopen the leak this
// contract exists to close, one layer down.
#AbsPath: =~"^/[^$]*$"

// #SecretUse — what a `use:` entry on a secret carries. DESIGN.cue:474-480.
//
// `src` is the host file the value is read from; `dst` is where it lands in the venue. Both are
// optional at the SCHEMA level because the delivery is resolved by the type owner, but the pair is
// what makes delivery possible without the value ever being text.
//
// `optional` is the one behavioural flag: a missing source does not fail the deployment. It exists
// because "this deployment works without the credential" is a real and common intent, and the
// alternative -- a placeholder secret authored to satisfy a required field -- is exactly how a real
// credential ends up in an authored file.
#SecretUse: {
	src?:      #AbsPath
	dst?:      #AbsPath
	optional?: bool
}
