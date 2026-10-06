package exec

// venue_envd.go — the machine-venue env.d PREAMBLE (charly#814).
//
// A candy's `env:` block compiles to `~/.config/opencharly/env.d/<candy>.env`,
// sourced by a managed block charly writes into the venue's shell INIT file
// (~/.bashrc). bash sources that file only for an INTERACTIVE shell, so every
// NON-interactive command a deploy or check runs on a machine venue
// (`target: local`, `target: vm`) — the deploy walk's rendered `run:`/`check:`
// scripts, the check engine's `command:` probe, and `charly vm ssh <box> --
// '<cmd>'` — ran WITHOUT the candy's env, even though the env file was written
// correctly. The image/pod path is unaffected: there the env is a container
// `ENV`, present to every process.
//
// The fix prefixes each rendered non-interactive command with MachineVenuePreamble:
// a POSIX-sh loop that sources the venue user's own env.d directory. `$HOME` is
// expanded on the VENUE (the snippet runs there), so one constant serves every
// machine venue. A glob that matches nothing leaves the loop body unrun (the
// `[ -r ]` guard), so a venue with no env.d is unaffected. Semicolon-terminated so
// a caller can concatenate a command after it.
//
// It lives here, NOT hard-coded into a leaf executor body, because both the
// deploy walk and the check engine converge on these leaf executors: prefixing
// the shared non-interactive spawn helpers makes the SAME env reachable to a
// `check:` probe and a deploy `run:` at once (R3 — one rule, all call sites).
const MachineVenuePreamble = `for f in "$HOME"/.config/opencharly/env.d/*.env; do [ -r "$f" ] && . "$f"; done; `

// WithMachineVenuePreamble prefixes script with MachineVenuePreamble. A blank
// script is returned unchanged (a no-op step pays nothing).
func WithMachineVenuePreamble(script string) string {
	if script == "" {
		return script
	}
	return MachineVenuePreamble + script
}
