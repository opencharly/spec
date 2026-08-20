package spec

import "sort"

// distro_methods.go — pure Go METHODS on the CUE-generated ResolvedDistro
// (sdk/schema/distro.cue -> spec/cue_types_gen.go). `cue exp gengotypes` has no
// construct for a method — it generates ONLY the field shape — so these stay
// hand-written here, mirroring Op.Kind() in spec/charly_methods.go: a method,
// not a type. Ported verbatim from the former hand-written ResolvedDistro
// (sdk/spec/distro_wire.go, deleted by the SDD conversion).
//
// The bodies are SHARED with the *Distro twins in spec/charly_methods.go via the
// unexported helpers below (R3 — one implementation, two receivers): Distro and
// ResolvedDistro are field-identical (Format map[string]*Format), so the format
// selection logic lives once.

// PrimaryFormat returns the distro's primary (non-secondary) build format —
// the deterministic first non-secondary Format name.
func (d *ResolvedDistro) PrimaryFormat() string {
	if d == nil {
		return ""
	}
	return primaryFormat(d.Format)
}

// LocalPkgFormat picks the format whose local_pkg block drives the charly
// package INSTALL (the download leg): the caller's primary format, then
// PrimaryFormat, then any localpkg-capable format.
func (d *ResolvedDistro) LocalPkgFormat(primaryFormat string) (string, *LocalPkg) {
	if d == nil {
		return "", nil
	}
	return localPkgFormat(d.Format, primaryFormat)
}

// primaryFormat is the shared format-selection core: the deterministic first
// non-secondary Format name (nil-safe on the map).
func primaryFormat(format map[string]*Format) string {
	names := make([]string, 0, len(format))
	for name := range format {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if fd := format[name]; fd != nil && fd.Secondary {
			continue // secondary build format (declared in YAML), never primary
		}
		return name
	}
	return ""
}

// localPkgFormat is the shared local-pkg selection core: the caller's primary
// format, then the distro's own primary, then any localpkg-capable format
// (deterministic).
func localPkgFormat(format map[string]*Format, callerPrimary string) (string, *LocalPkg) {
	for _, fmtName := range []string{callerPrimary, primaryFormat(format)} {
		if fmtName == "" {
			continue
		}
		if fd := format[fmtName]; fd != nil && fd.LocalPkg != nil {
			return fmtName, fd.LocalPkg
		}
	}
	names := make([]string, 0, len(format))
	for name := range format {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if fd := format[name]; fd != nil && fd.LocalPkg != nil {
			return name, fd.LocalPkg
		}
	}
	return "", nil
}
