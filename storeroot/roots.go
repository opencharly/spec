// Package storeroot resolves charly's FOUR roots — configuration, cache, state and
// runtime — and owns the single environment override that relocates the cache.
//
// # Why the roots are a CONTRACT, not a mechanism
//
// A root is not an implementation detail: the host and every plugin it serves must
// agree on WHERE the shared trees are, or a plugin writes its cache into a tree the
// host never sweeps and the host looks for a store the plugin never wrote. That
// agreement is a contract between two separately-built processes, so it lives on the
// contract side (this module), and it is the reason a plugin may import it just as
// freely as the host does.
//
// The placement is forced by the host's own boundary law, not chosen for taste: core
// imports only permitted contract surfaces (spec/* + proto) and may never import an
// sdk mechanism kit, while `sdk/storekit` — where these roots used to be computed —
// is a mechanism kit. Leaving the ONE resolver there gave the host exactly two
// options, hand-build the paths (which is what it did: `charlyDaemonPids` composed
// `$XDG_CACHE_HOME/charly` and `$XDG_RUNTIME_DIR/charly` itself, and so could not see
// `CHARLY_CACHE_DIR` at all) or import across the boundary it is forbidden to cross.
// Resolving here satisfies both rules as written: the host reaches the ONE resolver
// through a permitted surface, and the mechanism kits re-export it rather than keeping
// a second copy (see `sdk/storekit`).
//
// # The one override
//
// `CHARLY_CACHE_DIR` relocates the CACHE root, and only that root: it is the single
// environment override in the whole system, and every named store lives under it. The
// value is used exactly as the operator wrote it — the resolver never absolutises,
// cleans or rewrites it, so a relative value stays relative. No other root has an
// override.
//
// # This package resolves paths; it does not own the store
//
// The content-addressed store, its entries, views and garbage collection are the sdk
// mechanism layer's (`sdk/storekit`, which re-exports the symbols below). The legacy
// config-root store that `spec/cache` still carries resolves a DIFFERENT root
// (`opencharly/spec#233`) and is not served by this package.
package storeroot

import (
	"fmt"
	"os"
	"path/filepath"
)

// Root names one of the four roots.
type Root string

const (
	// RootConfig — `$XDG_CONFIG_HOME/charly`: what the USER sets.
	RootConfig Root = "config"
	// RootCache — `$XDG_CACHE_HOME/charly`: everything content-addressed and
	// re-derivable. The single `CHARLY_CACHE_DIR` override relocates this one and
	// therefore every store.
	RootCache Root = "cache"
	// RootState — `$XDG_STATE_HOME/charly`: durable state that is NOT cache — install
	// ledgers, process locks, anything a restart must not lose and a `cache clear`
	// must not touch.
	RootState Root = "state"
	// RootRuntime — `$XDG_RUNTIME_DIR/charly`: per-boot runtime state.
	RootRuntime Root = "runtime"
)

// Bases holds the five XDG Base Directory Specification values resolved for the current
// user. Dir and CacheDir build charly's own roots from these; hostenv expands an
// operator-authored `$XDG_*` token from the SAME values, so a relocated base moves both
// charly's trees and every deployment path.
type Bases struct {
	Data    string
	Config  string
	Cache   string
	State   string
	Runtime string
	// RuntimeSet reports whether `XDG_RUNTIME_DIR` was set to a VALID (absolute) path.
	// When false, Runtime carries the D-STORE-1 state fallback; the flag lets a caller
	// distinguish the two without re-reading the variable.
	RuntimeSet bool
}

// XDG resolves the XDG bases ONCE. It applies the specification's Unix defaults and its
// rule that a non-absolute value is invalid and MUST be ignored. The error is non-nil only
// when at least one base had to fall back to a RELATIVE default because `$HOME` was
// unavailable while its variable was unset or relative; the returned Bases still carry
// every value that did resolve. A caller that needs one base should use it when it is
// absolute and treat the home error as fatal only for the base that fell back.
func XDG() (Bases, error) {
	home, homeErr := os.UserHomeDir()
	unresolved := false
	base := func(env, defaultRel string) string {
		if v := os.Getenv(env); filepath.IsAbs(v) {
			return v
		}
		if homeErr != nil {
			unresolved = true
			return defaultRel
		}
		return filepath.Join(home, defaultRel)
	}
	// Runtime follows D-STORE-1: a valid (absolute) XDG_RUNTIME_DIR names it; otherwise the
	// runtime root falls back to the STATE base, never to /tmp. RuntimeSet tells a caller
	// whether the variable was genuinely set.
	bases := Bases{
		Data:   base("XDG_DATA_HOME", ".local/share"),
		Config: base("XDG_CONFIG_HOME", ".config"),
		Cache:  base("XDG_CACHE_HOME", ".cache"),
		State:  base("XDG_STATE_HOME", ".local/state"),
	}
	bases.Runtime = bases.State
	if v := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(v) {
		bases.Runtime = v
		bases.RuntimeSet = true
	}
	if unresolved {
		return bases, fmt.Errorf("resolve XDG bases: %w", homeErr)
	}
	return bases, nil
}

// Dir resolves one of the four roots. It is THE resolver: no caller builds a `.cache`
// path by hand.
func Dir(r Root) (string, error) {
	switch r {
	case RootCache:
		return CacheDir()
	case RootConfig:
		bases, homeErr := XDG()
		if !filepath.IsAbs(bases.Config) {
			return "", fmt.Errorf("resolve config root: %w", homeErr)
		}
		return filepath.Join(bases.Config, "charly"), nil
	case RootState:
		bases, homeErr := XDG()
		if !filepath.IsAbs(bases.State) {
			return "", fmt.Errorf("resolve state root: %w", homeErr)
		}
		return filepath.Join(bases.State, "charly"), nil
	case RootRuntime:
		// XDG() already resolves Runtime to the absolute variable or the D-STORE-1 state
		// fallback, so both consumers (this root and hostenv.ExpandHostHome) agree.
		bases, homeErr := XDG()
		if !filepath.IsAbs(bases.Runtime) {
			return "", fmt.Errorf("resolve runtime root: %w", homeErr)
		}
		return filepath.Join(bases.Runtime, "charly"), nil
	default:
		return "", fmt.Errorf("unknown root %q (want %s|%s|%s|%s)", r, RootConfig, RootCache, RootState, RootRuntime)
	}
}

// CacheDir resolves the cache root: `$XDG_CACHE_HOME/charly`, or `CHARLY_CACHE_DIR`
// when set. Every named store lives under it.
func CacheDir() (string, error) {
	if root := os.Getenv("CHARLY_CACHE_DIR"); root != "" {
		return root, nil
	}
	bases, homeErr := XDG()
	if !filepath.IsAbs(bases.Cache) {
		return "", fmt.Errorf("resolve cache root: %w", homeErr)
	}
	return filepath.Join(bases.Cache, "charly"), nil
}
