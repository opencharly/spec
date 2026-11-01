package spec

import (
	"testing"

	"cuelang.org/go/cue"
)

// ref_grammar_cue_test.go — the ref grammar's closure and ITS ONE PAIRING INVARIANT.
//
// Two things need pinning, and neither is visible to a Go-level round-trip:
//
//  1. The arms are CLOSED. `#CandyRef` used to be `^(@.+|bare)$`, so `@oci:/x` and
//     `@s3://bucket/x` validated and only failed later, as a nonsense git URL. A test
//     that decodes a valid ref would pass identically with the catch-all restored — every
//     reject case below FAILS if the arm reopens.
//  2. The two SPELLINGS of the remote arms agree. CUE's regex-only vocabulary cannot
//     interpolate `#GithubRef`/`#OciRef` (the SOURCE shapes, no `@`) into `#CandyRef`
//     (the REF-SITE spelling, with `@`), so the arm text is necessarily written twice.
//     Rather than hope the copies stay in step, TestRefGrammarsAgreeWithTheirSourceShapes
//     UNIFIES them against each other: for every sample, `#PluginSource` accepts exactly
//     the samples whose `@`-prefixed spelling `#CandyRef` accepts. A drift in either copy
//     fails that test, which is what makes the duplication safe (R2).
//
// Both defs are read from the SHIPPING concatenation (schemaconcat.ConcatSchema, the same
// contract the runtime and `charly task cue-gen` use), via the helper the sibling CUE test
// already provides — one loader, not a second hand-written excerpt of the schema.

// defAccepts reports whether the compiled schema's `def` accepts the CUE expression
// `expr` — the one unifier every accept/reject case below goes through, so a scalar case
// and a struct case differ only by how the value is spelled.
func defAccepts(t *testing.T, schema cue.Value, def, expr string) error {
	t.Helper()
	d := schema.LookupPath(cue.ParsePath(def))
	if err := d.Err(); err != nil {
		t.Fatalf("lookup %s: %v", def, err)
	}
	val := schema.Context().CompileString(expr)
	if err := val.Err(); err != nil {
		t.Fatalf("compile value %s: %v", expr, err)
	}
	return d.Unify(val).Validate(cue.Concrete(false), cue.Final())
}

// refAccepts is defAccepts for a concrete STRING (the ref arms are all string shapes).
func refAccepts(t *testing.T, schema cue.Value, def, s string) error {
	t.Helper()
	return defAccepts(t, schema, def, quoteCUE(s))
}

// quoteCUE renders s as a CUE string literal for CompileString.
func quoteCUE(s string) string {
	out := make([]rune, 0, len(s)+2)
	out = append(out, '"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			out = append(out, '\\', r)
		default:
			out = append(out, r)
		}
	}
	return string(append(out, '"'))
}

// The SOURCE shapes: what `plugin.source:` (and a requirement's `source:`) accepts.
func TestPluginSourceArmsAreClosed(t *testing.T) {
	schema := loadVmSchema(t)

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		{"git module accepted", "github.com/opencharly/plugin-bpf/candy/plugin-bpf", false},
		{"git module without sub-path accepted", "github.com/opencharly/plugin-bpf", false},
		{"registry ref accepted", "oci://ghcr.io/opencharly/plugin-bpf", false},
		{"registry ref with tag accepted", "oci://ghcr.io/opencharly/plugin-bpf:2026.286.0739", false},
		{"registry ref with port and tag accepted", "oci://127.0.0.1:5000/opencharly/plugin-bpf:v1", false},
		{"registry ref with digest accepted",
			"oci://ghcr.io/opencharly/plugin-bpf@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", false},
		{"registry ref with tag and digest accepted",
			"oci://ghcr.io/opencharly/plugin-bpf:v1@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", false},

		// A SOURCE field carries no `@` — that prefix belongs to a ref SITE (#CandyRef).
		// Accepting it here would make one ref mean two things depending on the field.
		{"ref-site spelling rejected in a source field", "@oci://ghcr.io/opencharly/plugin-bpf", true},
		{"at-prefixed git rejected in a source field", "@github.com/opencharly/plugin-bpf", true},
		// The schemes that used to slip through: they are the whole reason this test exists.
		{"s3 scheme rejected", "s3://bucket/prefix", true},
		{"ftp scheme rejected", "ftp://host/x", true},
		{"malformed oci scheme rejected", "oci:/x", true},
		{"unprefixed oci host rejected", "ghcr.io/opencharly/plugin-bpf", true},
		{"truncated git host rejected", "github.com", true},
		{"bare candy name rejected as a source", "plugin-bpf", true},
		{"empty rejected", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := refAccepts(t, schema, "#PluginSource", tc.value)
			if tc.reject && got == nil {
				t.Errorf("#PluginSource ACCEPTED %q — the source arms are not closed", tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("#PluginSource rejected the valid source %q: %v", tc.value, got)
			}
		})
	}
}

// The REF-SITE spelling: what require:/candy:/bake_plugin:/add_candy: accept.
func TestCandyRefArmsAreClosed(t *testing.T) {
	schema := loadVmSchema(t)

	for _, tc := range []struct {
		name   string
		value  string
		reject bool
	}{
		{"bare name accepted", "plugin-bpf", false},
		{"bare hyphenated name accepted", "layer-with-many-parts", false},
		{"git ref accepted", "@github.com/opencharly/plugin-bpf/candy/plugin-bpf:v1", false},
		{"git ref without version accepted", "@github.com/opencharly/plugin-bpf/candy/plugin-bpf", false},
		{"registry ref accepted", "@oci://ghcr.io/opencharly/plugin-bpf:v1", false},
		{"registry ref with digest accepted",
			"@oci://ghcr.io/opencharly/plugin-bpf@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", false},

		// The former `@.+` catch-all accepted every one of these.
		{"s3 ref rejected", "@s3://bucket/prefix", true},
		{"ftp ref rejected", "@ftp://host/x", true},
		{"file ref rejected", "@file:///etc/passwd", true},
		{"malformed oci ref rejected", "@oci:/x", true},
		{"unknown scheme rejected", "@foo://bar", true},
		{"bare at sign rejected", "@", true},
		{"empty rejected", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := refAccepts(t, schema, "#CandyRef", tc.value)
			if tc.reject && got == nil {
				t.Errorf("#CandyRef ACCEPTED %q — the ref arms are not closed", tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("#CandyRef rejected the valid ref %q: %v", tc.value, got)
			}
		})
	}
}

// TestRefGrammarsAgreeWithTheirSourceShapes is the PAIRING INVARIANT, and the reason the
// two spellings may be written twice: a SOURCE shape and its `@`-prefixed ref-site spelling
// must accept exactly the same set. Unifying the two copies against each other turns a
// textual duplication into a checked one — widen one arm and forget the other, and this
// fails immediately.
func TestRefGrammarsAgreeWithTheirSourceShapes(t *testing.T) {
	schema := loadVmSchema(t)

	for _, source := range []string{
		"github.com/opencharly/plugin-bpf",
		"github.com/opencharly/plugin-bpf/candy/plugin-bpf",
		"oci://ghcr.io/opencharly/plugin-bpf",
		"oci://ghcr.io/opencharly/plugin-bpf:v1",
		"oci://127.0.0.1:5000/opencharly/plugin-bpf:v1",
		"oci://ghcr.io/opencharly/plugin-bpf@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
		// Negatives: rejected in the source spelling must be rejected in the ref-site
		// spelling too, or the ref site becomes the looser of the two.
		"s3://bucket/prefix",
		"oci:/x",
		"github.com",
		"plugin-bpf",
		"",
	} {
		t.Run(source, func(t *testing.T) {
			sourceErr := refAccepts(t, schema, "#PluginSource", source)
			refErr := refAccepts(t, schema, "#CandyRef", "@"+source)

			if (sourceErr == nil) != (refErr == nil) {
				t.Errorf("the two spellings disagree for %q:\n  #PluginSource accepts it: %v (%v)\n  #CandyRef accepts @%s: %v (%v)",
					source, sourceErr == nil, sourceErr, source, refErr == nil, refErr)
			}
		})
	}
}

// TestPluginArtifactIdentityShape pins the identity facts the DECLARATION and the artifact
// CONFIG carry — the fields whose absence was measured to make a wrong binary run silently
// (`#Plugin` had neither a release nor a digest), and the one platform fact image-spec
// cannot express (libc). Every reject case fails if the field goes back to an unconstrained
// string or disappears.
// testPluginDigest is the sha256 of `{}` — the image-spec empty-descriptor digest, used
// here purely as a well-formed 64-hex digest. One value, one home: a second copy would
// make metadataCUE's own parameter "always the same argument" (unparam) and drift silently.
const testPluginDigest = "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"

func TestPluginArtifactIdentityShape(t *testing.T) {
	schema := loadVmSchema(t)
	digest := testPluginDigest

	for _, tc := range []struct {
		name   string
		def    string
		value  string
		reject bool
	}{
		// #Plugin.digest is the INDEX digest: a well-formed sha256, nothing else.
		{"plugin digest accepted", "#Plugin", `{providers: ["command:bpf"], source: "github.com/opencharly/plugin-bpf/candy/plugin-bpf", digest: "` + digest + `"}`, false},
		{"plugin digest short hex rejected", "#Plugin", `{providers: ["command:bpf"], source: "github.com/opencharly/plugin-bpf/candy/plugin-bpf", digest: "sha256:deadbeef"}`, true},
		{"plugin digest tag rejected", "#Plugin", `{providers: ["command:bpf"], source: "github.com/opencharly/plugin-bpf/candy/plugin-bpf", digest: "v1.2.3"}`, true},

		// #Plugin.release names the release; an empty pin is not a pin.
		{"plugin release accepted", "#Plugin", `{providers: ["command:bpf"], source: "github.com/opencharly/plugin-bpf/candy/plugin-bpf", release: "2026.286.0739"}`, false},
		{"plugin release empty rejected", "#Plugin", `{providers: ["command:bpf"], source: "github.com/opencharly/plugin-bpf/candy/plugin-bpf", release: ""}`, true},

		// #PluginArtifactMetadata.platform.libc — a closed two-value set, because
		// image-spec's platform object cannot carry it and a free string would let a
		// publisher invent a value no consumer can honour.
		{"metadata libc glibc accepted", "#PluginArtifactMetadata", metadataCUE(`"glibc"`), false},
		{"metadata libc musl accepted", "#PluginArtifactMetadata", metadataCUE(`"musl"`), false},
		{"metadata libc invented rejected", "#PluginArtifactMetadata", metadataCUE(`"uclibc"`), true},
		{"metadata libc empty rejected", "#PluginArtifactMetadata", metadataCUE(`""`), true},

		// served_schema_digest is REQUIRED: an artifact whose schema identity is absent
		// cannot be checked before splicing, which is the whole reason the field exists.
		{"metadata without served_schema_digest rejected", "#PluginArtifactMetadata",
			`{module: "m", revision: "r", go_sum: "g", toolchain: "go1.26.0", platform: {os: "linux", architecture: "amd64", libc: "glibc"}, protocol: 2, calver: "c", runtime_requires: [], providers: ["command:bpf"], layers: {}}`, true},

		// protocol / calver are OPTIONAL ADVISORY DIAGNOSTICS (DESIGN §8: no version
		// negotiation — compatibility is per-message validation, D-PROTO-6). A metadata that
		// omits them is a VALID artifact, and one that carries them is not thereby gated:
		// requiring them would over-constrain publishers for no safety gain, and would invite
		// the false impression that they decide anything (a `9999.999.9999` binary was measured
		// running fine).
		{"metadata without protocol/calver accepted as diagnostics", "#PluginArtifactMetadata",
			`{module: "m", revision: "r", go_sum: "g", toolchain: "go1.26.0", platform: {os: "linux", architecture: "amd64", libc: "glibc"}, served_schema_digest: "` + testPluginDigest + `", runtime_requires: [], providers: ["command:bpf"], layers: {binary: "sha256:00"}}`, false},

		// runtime_requires is REQUIRED and an empty list is a CLAIM, not a placeholder: the
		// measured harm was a provider whose tool dependency nobody declared, so its report
		// silently degraded to N/A. An artifact that omits the field is unfinished, and a
		// consumer must never read its absence as "needs nothing".
		{"metadata without runtime_requires rejected", "#PluginArtifactMetadata", metadataCUENoRuntime(digest), true},
		{"metadata empty runtime_requires accepted as a claim", "#PluginArtifactMetadata", metadataCUE(`"glibc"`), false},
		{"metadata runtime_requires with a tool accepted", "#PluginArtifactMetadata",
			`{module: "m", revision: "r", go_sum: "g", toolchain: "go1.26.0", platform: {os: "linux", architecture: "amd64", libc: "glibc"}, protocol: 2, calver: "c", served_schema_digest: "` + digest + `", runtime_requires: ["zcat"], providers: ["command:bpf"], layers: {binary: "sha256:00"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := defAccepts(t, schema, tc.def, tc.value)
			if tc.reject && got == nil {
				t.Errorf("%s ACCEPTED %s — the field is not constrained", tc.def, tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("%s rejected a valid value %s: %v", tc.def, tc.value, got)
			}
		})
	}
}

// metadataCUE renders a complete, well-formed #PluginArtifactMetadata body with the given
// libc value — the accept cases vary ONLY libc, so the rest is fixed here.
func metadataCUE(libc string) string {
	return `{module: "m", revision: "r", go_sum: "g", toolchain: "go1.26.0", ` +
		`platform: {os: "linux", architecture: "amd64", libc: ` + libc + `}, ` +
		`protocol: 2, calver: "c", served_schema_digest: "` + testPluginDigest + `", ` +
		`runtime_requires: [], providers: ["command:bpf"], layers: {binary: "sha256:00"}}`
}

// metadataCUENoRuntime is metadataCUE minus runtime_requires — the field the measured harm
// says must not be omissible.
func metadataCUENoRuntime(servedDigest string) string {
	return `{module: "m", revision: "r", go_sum: "g", toolchain: "go1.26.0", ` +
		`platform: {os: "linux", architecture: "amd64", libc: "glibc"}, ` +
		`protocol: 2, calver: "c", served_schema_digest: "` + servedDigest + `", ` +
		`providers: ["command:bpf"], layers: {binary: "sha256:00"}}`
}
