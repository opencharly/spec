package spec

// render_seam.go — the render-seam METHOD discriminators (the RenderSeamRequest.Method
// values) for the two render-seam methods that still need a host callback
// (inline-builder, ensure-builders). Relocated from sdk/deploykit/render_seam.go
// (#55 import-purity cone-render) so charly's host-builder + candy/plugin-build's
// producer share ONE source without a deploykit import. The per-method param/result
// structs (#InlineBuilderParams / #InlineBuilderResult / #EnsureBuildersParams) are
// CUE-sourced (schema/buildresolve.cue) like the outer #RenderSeamRequest itself.
const (
	RenderSeamInlineBuilder  = "inline-builder"
	RenderSeamEnsureBuilders = "ensure-builders"
)
