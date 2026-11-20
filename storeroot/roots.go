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
	"strconv"
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
	// Dir(RootRuntime) uses it to choose between the runtime root and the documented
	// state-root fallback; XDG().Runtime always carries a path, so a caller must not
	// infer availability from the path alone.
	RuntimeSet bool
}

// XDG resolves the XDG bases ONCE. It applies the specification's Unix defaults and its
// rule that a non-absolute value is invalid and MUST be ignored. The error is non-nil
// only when no base can be resolved at all ($HOME unavailable and the variable unset);
// the returned Bases still carry the relative defaults.
func XDG() (Bases, error) {
	home, homeErr := os.UserHomeDir()
	base := func(env, defaultRel string) string {
		if v := os.Getenv(env); filepath.IsAbs(v) {
			return v
		}
		if homeErr != nil {
			return defaultRel
		}
		return filepath.Join(home, defaultRel)
	}
	runtime := ""
	runtimeSet := false
	if v := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(v) {
		runtime = v
		runtimeSet = true
	} else {
		runtime = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	}
	bases := Bases{
		Data:       base("XDG_DATA_HOME", ".local/share"),
		Config:     base("XDG_CONFIG_HOME", ".config"),
		Cache:      base("XDG_CACHE_HOME", ".cache"),
		State:      base("XDG_STATE_HOME", ".local/state"),
		Runtime:    runtime,
		RuntimeSet: runtimeSet,
	}
	if homeErr != nil {
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
		// The runtime path does not depend on $HOME, so a valid XDG_RUNTIME_DIR answers
		// even when XDG() reports that $HOME is unavailable. An unset or non-absolute
		// value is invalid (the specification's rule), and the root falls back to STATE
		// rather than /tmp, which is shared and world-writable (D-STORE-1).
		bases, _ := XDG()
		if bases.RuntimeSet {
			return filepath.Join(bases.Runtime, "charly"), nil
		}
		return Dir(RootState)
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
