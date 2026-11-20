// Package storeroot resolves charly's roots — configuration, data, cache, state and
// runtime — and owns their charly-only overrides, the environment that relocates a root
// for charly alone.
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
// # The charly-only overrides
//
// Every XDG input a charly root is built from has a charly-only override that takes
// precedence for charly over that input (D-STORE-1): `CHARLY_CONFIG_HOME`,
// `CHARLY_CONFIG_DIRS`, `CHARLY_DATA_HOME`, `CHARLY_DATA_DIRS`, `CHARLY_STATE_HOME`,
// `CHARLY_CACHE_HOME` and `CHARLY_RUNTIME_DIR`. The `_HOME`/`_DIR` form names a
// COMPLETE charly root — an absolute path, not an XDG base — and the `_DIRS` form names
// a colon-separated list of complete charly directories. An override never moves the
// XDG base itself, so a path belonging to another application (a quadlet unit, a fish
// snippet, libvirt) keeps resolving against `$XDG_*`; only charly's own trees move.
// A relative or empty override is invalid and is ignored, so the XDG input's default
// applies. When both cache overrides are set, `CHARLY_CACHE_HOME` wins.
//
// `CHARLY_CACHE_DIR` and `CHARLY_REPO_CACHE` remain compatibility overrides and are
// never dropped (operator direction of 2026-11-20): `CHARLY_CACHE_DIR` relocates the
// cache root and is used exactly as the operator wrote it — never absolutised, cleaned
// or rewritten, so a relative value stays relative — and `CHARLY_REPO_CACHE` names the
// remote-repo directory (see `refs`).
//
// # Cache and data are different trees
//
// The cache root is re-derivable and holds the one content-addressed store (D-STORE-3).
// The data root holds durable user data that cannot be re-derived — SSH keys, VM disks
// and per-VM state, encrypted-volume plaintext, bind-mounted volumes (D-STORE-12). A
// built image or artifact is a cached item and stays in the cache store, and `charly
// clean` never evicts the data root (D-STORE-7).
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
	"strings"
)

// Root names one of the roots.
type Root string

const (
	// RootConfig — `$XDG_CONFIG_HOME/charly`: what the USER sets.
	RootConfig Root = "config"
	// RootData — `$XDG_DATA_HOME/charly`: durable user data that cannot be
	// re-derived. It is NOT a cache: a cached item stays in the cache store, and
	// `charly clean` never evicts this root.
	RootData Root = "data"
	// RootCache — `$XDG_CACHE_HOME/charly`: everything content-addressed and
	// re-derivable. `CHARLY_CACHE_HOME` (then the compatibility `CHARLY_CACHE_DIR`)
	// relocates this root and therefore every store.
	RootCache Root = "cache"
	// RootState — `$XDG_STATE_HOME/charly`: durable state that is NOT cache — install
	// ledgers, process locks, anything a restart must not lose and a `cache clear`
	// must not touch.
	RootState Root = "state"
	// RootRuntime — `$XDG_RUNTIME_DIR/charly`: per-boot runtime state.
	RootRuntime Root = "runtime"
)

// Bases holds the XDG Base Directory Specification values resolved for the current
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

// Resolver is the D-STORE-8 test seam: it resolves the same roots from injected base
// values and an injected environment lookup, so a table test never depends on the
// process environment. Default() wires it to XDG() and os.LookupEnv; production callers
// use the package-level functions, which delegate here.
type Resolver struct {
	Bases     func() (Bases, error)
	LookupEnv func(string) (string, bool)
}

// Default returns the process-environment resolver.
func Default() *Resolver {
	return &Resolver{Bases: XDG, LookupEnv: os.LookupEnv}
}

func (r *Resolver) bases() (Bases, error) {
	if r != nil && r.Bases != nil {
		return r.Bases()
	}
	return XDG()
}

func (r *Resolver) lookupEnv(key string) (string, bool) {
	if r != nil && r.LookupEnv != nil {
		return r.LookupEnv(key)
	}
	return os.LookupEnv(key)
}

// The charly-only override for each root (D-STORE-1). Each names a COMPLETE charly root,
// so the resolver uses it verbatim instead of joining `charly` onto an XDG base.
var rootOverride = map[Root]string{
	RootConfig:  "CHARLY_CONFIG_HOME",
	RootData:    "CHARLY_DATA_HOME",
	RootState:   "CHARLY_STATE_HOME",
	RootCache:   "CHARLY_CACHE_HOME",
	RootRuntime: "CHARLY_RUNTIME_DIR",
}

// rootSearchOverride names the charly-only search-path override of a root that has one.
var rootSearchOverride = map[Root]string{
	RootConfig: "CHARLY_CONFIG_DIRS",
	RootData:   "CHARLY_DATA_DIRS",
}

// override returns the root's charly-only override when it is set to a valid value: an
// absolute COMPLETE charly root. A relative or empty value is invalid and ignored.
func (r *Resolver) override(root Root) (string, bool) {
	key, ok := rootOverride[root]
	if !ok {
		return "", false
	}
	if v, ok := r.lookupEnv(key); ok && v != "" && filepath.IsAbs(v) {
		return v, true
	}
	return "", false
}

// searchOverride returns the root's charly-only search list when it holds at least one
// absolute entry. An entry that is not absolute is invalid and is ignored.
func (r *Resolver) searchOverride(root Root) ([]string, bool) {
	key, ok := rootSearchOverride[root]
	if !ok {
		return nil, false
	}
	v, ok := r.lookupEnv(key)
	if !ok || v == "" {
		return nil, false
	}
	dirs := searchDirs(v, nil)
	if len(dirs) == 0 {
		return nil, false
	}
	return dirs, true
}

// Base resolves the UNSURFIXED XDG base a root lives under. It is what a path belonging
// to another application (systemd/quadlet, libvirt, opencharly, fish) is built from; use
// Dir for charly's own trees. The charly-only overrides are COMPLETE charly roots, not
// XDG bases, so they do not move the base of any other application's path.
//
// RootCache is special, and only because the cache overrides replace that one root
// wholesale: `CHARLY_CACHE_HOME` and then `CHARLY_CACHE_DIR` are returned verbatim, so a
// charly-owned sibling of the cache root (the shared Go temp dir) lands beside it.
func (r *Resolver) Base(root Root) (string, error) {
	bases, err := r.bases()
	switch root {
	case RootConfig:
		return absoluteBase(bases.Config, err, "config")
	case RootData:
		return absoluteBase(bases.Data, err, "data")
	case RootState:
		return absoluteBase(bases.State, err, "state")
	case RootRuntime:
		return absoluteBase(bases.Runtime, err, "runtime")
	case RootCache:
		if v, ok := r.override(RootCache); ok {
			return v, nil
		}
		if v, ok := r.lookupEnv("CHARLY_CACHE_DIR"); ok && v != "" {
			return v, nil
		}
		return absoluteBase(bases.Cache, err, "cache")
	default:
		return "", unknownRoot(root)
	}
}

// Dir resolves one of the roots. It is THE resolver: no caller builds a `.config`,
// `.local/share` or `.cache` path by hand. A root's charly-only override is a complete
// charly root and is returned verbatim.
func (r *Resolver) Dir(root Root) (string, error) {
	if root == RootCache {
		return r.CacheDir()
	}
	if v, ok := r.override(root); ok {
		return v, nil
	}
	base, err := r.Base(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "charly"), nil
}

// CacheDir resolves the cache root: `CHARLY_CACHE_HOME` when set, else the
// `CHARLY_CACHE_DIR` compatibility override (verbatim), else `$XDG_CACHE_HOME/charly`.
// Every named store lives under it.
func (r *Resolver) CacheDir() (string, error) {
	if v, ok := r.override(RootCache); ok {
		return v, nil
	}
	if v, ok := r.lookupEnv("CHARLY_CACHE_DIR"); ok && v != "" {
		return v, nil
	}
	bases, err := r.bases()
	base, err := absoluteBase(bases.Cache, err, "cache")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "charly"), nil
}

// DataDir resolves the durable data root (`$XDG_DATA_HOME/charly`).
func (r *Resolver) DataDir() (string, error) { return r.Dir(RootData) }

// BaseDirs returns the RAW XDG search path for a root that has one (configuration or
// data), in specification order. An unset or empty variable takes the Unix defaults; a
// non-absolute entry is ignored. It reads the XDG input only: the charly-only `_DIRS`
// overrides name COMPLETE charly directories, so they are applied by Dirs, never here.
func (r *Resolver) BaseDirs(root Root) ([]string, error) {
	switch root {
	case RootConfig:
		value, _ := r.lookupEnv("XDG_CONFIG_DIRS")
		return searchDirs(value, []string{"/etc/xdg"}), nil
	case RootData:
		value, _ := r.lookupEnv("XDG_DATA_DIRS")
		return searchDirs(value, []string{"/usr/local/share", "/usr/share"}), nil
	default:
		return nil, fmt.Errorf("root %q has no search path", root)
	}
}

// Dirs returns the root's search path: charly's own root first, then the search entries.
// A `CHARLY_CONFIG_DIRS`/`CHARLY_DATA_DIRS` override supplies complete charly
// directories and is used verbatim; otherwise each XDG search entry is read from its
// `charly` subdirectory. Only configuration and data have a search path.
func (r *Resolver) Dirs(root Root) ([]string, error) {
	if root != RootConfig && root != RootData {
		return nil, fmt.Errorf("root %q has no search path", root)
	}
	home, err := r.Dir(root)
	if err != nil {
		return nil, err
	}
	search, ok := r.searchOverride(root)
	if !ok {
		xdg, err := r.BaseDirs(root)
		if err != nil {
			return nil, err
		}
		search = make([]string, 0, len(xdg))
		for _, dir := range xdg {
			search = append(search, filepath.Join(dir, "charly"))
		}
	}
	out := make([]string, 0, 1+len(search))
	out = append(out, home)
	return append(out, search...), nil
}

// Search returns the first existing file named by rel across Dirs(root). rel must be
// relative and must not contain a `..` element. A missing file is ("", false, nil), not
// an error.
func (r *Resolver) Search(root Root, rel string) (string, bool, error) {
	if filepath.IsAbs(rel) {
		return "", false, fmt.Errorf("search path %q must be relative", rel)
	}
	for part := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		if part == ".." {
			return "", false, fmt.Errorf("search path %q must not contain a parent element", rel)
		}
	}
	dirs, err := r.Dirs(root)
	if err != nil {
		return "", false, err
	}
	for _, dir := range dirs {
		path := filepath.Join(dir, rel)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true, nil
		}
	}
	return "", false, nil
}

// Package-level delegates to the process-environment resolver.

// Dir resolves one of the roots through the process-environment resolver.
func Dir(root Root) (string, error) { return Default().Dir(root) }

// Base resolves the unsuffixed XDG base through the process-environment resolver.
func Base(root Root) (string, error) { return Default().Base(root) }

// CacheDir resolves the cache root through the process-environment resolver.
func CacheDir() (string, error) { return Default().CacheDir() }

// DataDir resolves the durable data root through the process-environment resolver.
func DataDir() (string, error) { return Default().DataDir() }

// BaseDirs returns the raw search path through the process-environment resolver.
func BaseDirs(root Root) ([]string, error) { return Default().BaseDirs(root) }

// Dirs returns the root search path through the process-environment resolver.
func Dirs(root Root) ([]string, error) { return Default().Dirs(root) }

// Search finds a relative file through the process-environment resolver.
func Search(root Root, rel string) (string, bool, error) { return Default().Search(root, rel) }

func absoluteBase(base string, err error, name string) (string, error) {
	if filepath.IsAbs(base) {
		return base, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s root: %w", name, err)
	}
	return "", fmt.Errorf("resolve %s root: %q is not absolute", name, base)
}

func searchDirs(value string, defaults []string) []string {
	var out []string
	for dir := range strings.SplitSeq(value, ":") {
		if filepath.IsAbs(dir) {
			out = append(out, dir)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), defaults...)
	}
	return out
}

func unknownRoot(root Root) error {
	return fmt.Errorf("unknown root %q (want %s|%s|%s|%s|%s)", root, RootConfig, RootData, RootCache, RootState, RootRuntime)
}
