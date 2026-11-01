package spec

// plugin_artifact_consts.go — the WIRE IDENTITY of a plugin artifact: the media types and
// annotation keys the producer (candy/plugin-oci) and every consumer (the sdk
// materialization/verification mechanism, charly core's resolution) must agree on BY NAME.
//
// One home (R2/R3): a media type transcribed into two repos drifts, and the drift is silent —
// a consumer looking for the old string simply finds no layer. The artifact itself is an
// image-spec v1.1.1 artifact manifest (the decision-tree "case 3": content blobs + JSON
// configuration), carried under an image index with one descriptor per platform.
//
// Naming rules this file obeys, from the spec's annotations.md:
//   - `org.opencontainers.*` is RESERVED for keys OCI defines. Every key used here is used
//     with its DEFINED meaning, and NO new `org.opencontainers.*` key is invented.
//   - everything charly-specific is namespaced `ai.opencharly.*`.
//   - a consumer MUST NOT error on an unknown annotation or an unknown media type.
//
// The media types are RFC 6838 §4.2-syntax-compliant vendor types (`application/vnd.<vendor>.
// <name>[.v<n>]`); none invents a structured-syntax `+suffix` that is not registered.
const (
	// MediaTypePluginArtifactManifest — the per-platform artifact manifest. The standard
	// image-manifest media type: an artifact manifest IS an image manifest (the artifactType
	// field is what makes it an artifact).
	MediaTypePluginArtifactManifest = "application/vnd.oci.image.manifest.v1+json"

	// MediaTypePluginArtifactIndex — the multi-platform index. Standard media type; the
	// artifactType below is set on it as well.
	MediaTypePluginArtifactIndex = "application/vnd.oci.image.index.v1+json"

	// ArtifactTypePlugin — what the artifact IS. Set on BOTH the index and each per-platform
	// manifest, so a consumer that has only one of the two still knows what it holds.
	ArtifactTypePlugin = "application/vnd.opencharly.plugin.v1"

	// MediaTypePluginConfig — the config blob's media type. Because it is a
	// plugin-specific value rather than `application/vnd.oci.image.config.v1+json`, the
	// spec requires `artifactType` to be set (it is), and requires a store/copy to treat
	// the blob as opaque rather than trying to parse it as a runtime config.
	MediaTypePluginConfig = "application/vnd.opencharly.plugin.config.v1+json"

	// MediaTypePluginBinary — the provider binary, as a RAW BLOB layer (mode 0755, no tar
	// wrapper). A store or copy that does not know this media type must carry it verbatim;
	// nothing needs to unpack a layer to move an artifact.
	MediaTypePluginBinary = "application/vnd.opencharly.plugin.binary.v1"

	// MediaTypePluginManifest — the candy's own charly.yml (`plugin:` block included), so a
	// consumer with no source checkout can still SCAN the candy and declare the plugin.
	MediaTypePluginManifest = "application/vnd.opencharly.plugin.manifest.v1+yaml"

	// MediaTypePluginProviders — the `.providers` word manifest, BYTE-IDENTICAL to the file
	// a `bake_plugin:` image bake writes (newline-joined `class:word[:parent]`). That
	// byte-identity is what lets materialization be a copy rather than a conversion, and
	// what makes the PROVEN baked connect path serve a pulled plugin unchanged.
	MediaTypePluginProviders = "application/vnd.opencharly.plugin.providers.v1"
)

// The layer ROLE names #PluginArtifactMetadata.layers is keyed by. Roles are how a consumer
// maps "the blob I verified" to "the file it writes"; the media types above identify the same
// three layers on the wire, and a role whose layer carries the wrong media type is an error.
const (
	PluginArtifactLayerBinary    = "binary"
	PluginArtifactLayerManifest  = "manifest"
	PluginArtifactLayerProviders = "providers"
)

// The standard annotations used with their DEFINED meanings (annotations.md). Nothing here is
// a charly invention: `version`, `revision`, `source`, `title`, `description`, `licenses`,
// `vendor`, `url` and `documentation` mean exactly what the spec says they mean.
//
// `org.opencontainers.image.created` is deliberately ABSENT and MUST NOT be added: it is a
// wall-clock fact, and a wall-clock annotation gives identical inputs a different digest —
// breaking the content-addressing everything else here rests on (D-STORE-3/D-STORE-4, and
// D-STORE-6's rule that a stored fact must be an input). If a time is ever wanted it must
// DERIVE from an input (SOURCE_DATE_EPOCH), never from `time.Now()`.
//
// `org.opencontainers.image.ref.name` is likewise not listed: the spec says it is only valid on
// descriptors in an image layout's index.json, so it belongs to the local layout store, never
// to a pushed index.
const (
	AnnotationVersion       = "org.opencontainers.image.version"
	AnnotationRevision      = "org.opencontainers.image.revision"
	AnnotationSource        = "org.opencontainers.image.source"
	AnnotationTitle         = "org.opencontainers.image.title"
	AnnotationDescription   = "org.opencontainers.image.description"
	AnnotationLicenses      = "org.opencontainers.image.licenses"
	AnnotationVendor        = "org.opencontainers.image.vendor"
	AnnotationURL           = "org.opencontainers.image.url"
	AnnotationDocumentation = "org.opencontainers.image.documentation"
)

// The charly-specific annotations — the `ai.opencharly.*` namespace, never
// `org.opencontainers.*`. These MIRROR the #PluginArtifactMetadata config blob for a reader
// that has the manifest but has not fetched the config (registry UIs, `charly box inspect`,
// a human with `crane manifest`). The config blob is the SOURCE; a mirror is derived from it
// and never independently authored (R2) — the two disagreeing is a bug in the producer.
const (
	AnnotationPluginCalver       = "ai.opencharly.plugin.calver"
	AnnotationPluginProtocol     = "ai.opencharly.plugin.protocol"
	AnnotationPluginLibc         = "ai.opencharly.plugin.libc"
	AnnotationPluginModule       = "ai.opencharly.plugin.module"
	AnnotationPluginToolchain    = "ai.opencharly.plugin.toolchain"
	AnnotationPluginSchemaDigest = "ai.opencharly.plugin.cue.schema.digest"
	AnnotationPluginProviders    = "ai.opencharly.plugin.providers"
)
