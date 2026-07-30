package spec

// externalized_builders.go — the D-FACT of which detection-builder words are served by an EXTERNAL
// out-of-process plugin (no in-proc BuilderProvider). #55 import-purity: relocated from sdk/buildkit
// DOWN to spec (the wire/value leaf) so charly core reads the ONE source over its spec+proto-only
// import surface; sdk/buildkit keeps a thin var-forwarder for its plugin callers.

// ExternalizedBuilders is THE single source of truth for which builder words are served by an EXTERNAL
// out-of-process plugin. A word here resolves through the registry to a *grpcProvider connected at
// plugin-load time.
var ExternalizedBuilders = map[string]bool{
	"cargo": true,
	"npm":   true,
	"pixi":  true,
	"aur":   true,
}
