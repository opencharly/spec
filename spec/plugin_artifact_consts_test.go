package spec

import (
	"strings"
	"testing"
)

// plugin_artifact_consts_test.go — pins the ANNOTATION/NAMESPACE LAW the artifact must obey,
// so a later "convenience" key cannot be added by accident.
//
// image-spec v1.1.1 annotations.md reserves `org.opencontainers.*` for keys OCI defines
// ("MUST NOT be used by other specifications and extensions"), and the `org.opencontainers.image`
// namespace twice over. The failure this guards is specific and easy to commit: reaching for
// `org.opencontainers.image.arch` (or similar) because it LOOKS standard, when the only legal
// home for a charly-invented fact is `ai.opencharly.*`.

// imageSpecAnnotationKeys is the DEFINED key list from image-spec v1.1.1 annotations.md
// ("Pre-Defined Annotation Keys"). This is the spec's table, transcribed once, here, as the
// membership oracle: it is not a copy of our constants (the test below holds those) but the
// external contract they must fall inside.
var imageSpecAnnotationKeys = map[string]bool{
	"org.opencontainers.image.created":       true,
	"org.opencontainers.image.authors":       true,
	"org.opencontainers.image.url":           true,
	"org.opencontainers.image.documentation": true,
	"org.opencontainers.image.source":        true,
	"org.opencontainers.image.version":       true,
	"org.opencontainers.image.revision":      true,
	"org.opencontainers.image.vendor":        true,
	"org.opencontainers.image.licenses":      true,
	"org.opencontainers.image.ref.name":      true,
	"org.opencontainers.image.title":         true,
	"org.opencontainers.image.description":   true,
	"org.opencontainers.image.base.digest":   true,
	"org.opencontainers.image.base.name":     true,
}

func TestPluginArtifactAnnotationsUseDefinedMeanings(t *testing.T) {
	ours := map[string]string{
		"AnnotationVersion":            AnnotationVersion,
		"AnnotationRevision":           AnnotationRevision,
		"AnnotationSource":             AnnotationSource,
		"AnnotationTitle":              AnnotationTitle,
		"AnnotationDescription":        AnnotationDescription,
		"AnnotationLicenses":           AnnotationLicenses,
		"AnnotationVendor":             AnnotationVendor,
		"AnnotationURL":                AnnotationURL,
		"AnnotationDocumentation":      AnnotationDocumentation,
		"AnnotationPluginCalver":       AnnotationPluginCalver,
		"AnnotationPluginProtocol":     AnnotationPluginProtocol,
		"AnnotationPluginLibc":         AnnotationPluginLibc,
		"AnnotationPluginModule":       AnnotationPluginModule,
		"AnnotationPluginToolchain":    AnnotationPluginToolchain,
		"AnnotationPluginSchemaDigest": AnnotationPluginSchemaDigest,
		"AnnotationPluginProviders":    AnnotationPluginProviders,
	}

	for name, key := range ours {
		switch {
		case strings.HasPrefix(key, "org.opencontainers."):
			// Legal ONLY with its defined meaning. An invented key here is the exact defect
			// the spec forbids, and it would be invisible at runtime — a consumer must
			// ignore an unknown annotation, so nothing would ever fail.
			if !imageSpecAnnotationKeys[key] {
				t.Errorf("%s = %q: `org.opencontainers.*` is reserved for keys image-spec defines; %q is not one of them — a charly-invented fact belongs under `ai.opencharly.*`", name, key, key)
			}
			// One defined meaning we deliberately do NOT use. This is a DECISION, not an
			// oversight: a wall-clock `created` gives identical inputs a different digest,
			// which defeats the content addressing the store keys on (D-STORE-3/4) and
			// D-STORE-6's rule that a stored fact must be an input. If a time is ever
			// wanted it must DERIVE from an input (SOURCE_DATE_EPOCH).
			if key == "org.opencontainers.image.created" {
				t.Errorf("%s is the wall-clock created key — the artifact must be reproducible from its inputs; omit it, or derive it from SOURCE_DATE_EPOCH, never from time.Now()", name)
			}
		case strings.HasPrefix(key, "ai.opencharly."):
			// Our namespace. Fine by construction; asserted so a typo'd namespace
			// (`ai.opencharly` vs `io.opencharly`) fails here rather than silently
			// producing an unreadable artifact.
		default:
			t.Errorf("%s = %q: every annotation key is either an image-spec `org.opencontainers.*` key or namespaced `ai.opencharly.*`", name, key)
		}
	}
}

// TestPluginArtifactMediaTypesAreVendorNamespaced keeps the media types RFC 6838 §4.2-shaped:
// `application/vnd.<vendor>.<name>[.v<n>]` for the plugin-specific ones, and the two standard
// OCI media types for the manifest/index. An unknown media type must never error a store or a
// copy, which is why the plugin-specific layers use a plain vendor type and not a
// structured-syntax `+suffix` that would imply a parser nobody has.
func TestPluginArtifactMediaTypesAreVendorNamespaced(t *testing.T) {
	for name, mt := range map[string]string{
		"MediaTypePluginConfig":    MediaTypePluginConfig,
		"MediaTypePluginBinary":    MediaTypePluginBinary,
		"MediaTypePluginManifest":  MediaTypePluginManifest,
		"MediaTypePluginProviders": MediaTypePluginProviders,
		"ArtifactTypePlugin":       ArtifactTypePlugin,
	} {
		if !strings.HasPrefix(mt, "application/vnd.opencharly.") {
			t.Errorf("%s = %q: a plugin-specific media type must be an `application/vnd.opencharly.*` vendor type", name, mt)
		}
	}
	for name, mt := range map[string]string{
		"MediaTypePluginArtifactManifest": MediaTypePluginArtifactManifest,
		"MediaTypePluginArtifactIndex":    MediaTypePluginArtifactIndex,
	} {
		if !strings.HasPrefix(mt, "application/vnd.oci.image.") {
			t.Errorf("%s = %q: the manifest and index use the STANDARD OCI media types — an artifact manifest is an image manifest", name, mt)
		}
	}
}
