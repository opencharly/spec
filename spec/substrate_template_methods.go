package spec

// substrate_template_methods.go — pure Go METHODS on the CUE-generated
// ResolvedAndroid (sdk/schema/substrate_template.cue -> spec/cue_types_gen.go).
// `cue exp gengotypes` has no construct for a method — it generates ONLY the
// field shape — so these stay hand-written here, mirroring Op.Kind() in
// spec/charly_methods.go: a method, not a type. Ported verbatim from the
// former hand-written ResolvedAndroid (sdk/spec/substrate_template_wire.go,
// deleted by the SDD conversion).
//
// The bodies are SHARED with the *Android twins in spec/charly_methods.go via
// the unexported helpers below (R3 — one implementation, two receivers):
// Android and ResolvedAndroid are field-identical (Serial string, Adb
// *AdbEndpoint), so the endpoint/serial logic lives once.

// IsEndpoint reports whether the resolved device targets a remote adb
// endpoint.
func (a *ResolvedAndroid) IsEndpoint() bool {
	return a != nil && isAndroidEndpoint(a.Adb)
}

// EffectiveSerial returns the device serial, defaulting to the emulator
// serial.
func (a *ResolvedAndroid) EffectiveSerial() string {
	if a == nil {
		return "emulator-5554"
	}
	return effectiveAndroidSerial(a.Serial)
}

// isAndroidEndpoint is the shared endpoint core: a remote adb endpoint is
// present exactly when the adb block names a host.
func isAndroidEndpoint(adb *AdbEndpoint) bool {
	return adb != nil && adb.Host != ""
}

// effectiveAndroidSerial is the shared serial core: the configured serial, or
// the emulator default.
func effectiveAndroidSerial(serial string) string {
	if serial != "" {
		return serial
	}
	return "emulator-5554"
}
