package container

// resolve_shell_image_ref_test.go — opencharly/spec#453: a requested tag REPLACES the tag
// (or digest) the name already carries, instead of compounding it.

import "testing"

func TestResolveShellImageRefReplacesAnExistingTag(t *testing.T) {
	cases := []struct{ registry, name, tag, want string }{
		// The filed defect: a full recorded ref plus a requested tag.
		{"", "ghcr.io/opencharly/whisper:2026.325.2318", "2026.332.0936", "ghcr.io/opencharly/whisper:2026.332.0936"},
		// A digest is replaced by the tag.
		{"", "ghcr.io/opencharly/whisper@sha256:abc123", "2026.332.0936", "ghcr.io/opencharly/whisper:2026.332.0936"},
		// A bare repo is unchanged by the strip.
		{"", "ghcr.io/opencharly/whisper", "2026.332.0936", "ghcr.io/opencharly/whisper:2026.332.0936"},
		// A registry PORT is not a tag.
		{"", "localhost:5000/opencharly/whisper", "2026.332.0936", "localhost:5000/opencharly/whisper:2026.332.0936"},
		{"", "localhost:5000/opencharly/whisper:2026.325.2318", "2026.332.0936", "localhost:5000/opencharly/whisper:2026.332.0936"},
		// The registry argument still composes as before.
		{"ghcr.io", "opencharly/whisper", "2026.332.0936", "ghcr.io/opencharly/whisper:2026.332.0936"},
	}
	for _, c := range cases {
		if got := ResolveShellImageRef(c.registry, c.name, c.tag); got != c.want {
			t.Fatalf("ResolveShellImageRef(%q, %q, %q) = %q, want %q", c.registry, c.name, c.tag, got, c.want)
		}
	}
}
