package spec

import "errors"

// image_errors.go — image-storage sentinel errors (RELOCATED from sdk/kit box_metadata.go,
// #55 value extraction). A pure error sentinel over the image E-envelope, returned by the
// host metadata probe (sdk/kit.ExtractMetadata) and matched by charly core via errors.Is;
// homed in spec so charly matches spec.ErrImageNotLocal without importing sdk/kit. sdk/kit
// re-exports it so existing kit.ErrImageNotLocal call sites are untouched.

// ErrImageNotLocal is returned (wrapped with the image ref) when an image is not present in
// local container storage — a host-side signal, never crossing the plugin wire.
var ErrImageNotLocal = errors.New("image not found in local storage")
