package spec

import (
	"strings"
	"testing"
)

func TestParseRemoteRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantRepo string
		wantSub  string
		wantName string
		wantVer  string
	}{
		{"@github.com/org/repo/layers/cuda:v1.0.0", "github.com/org/repo", "layers/cuda", "cuda", "v1.0.0"},
		{"@github.com/org/repo/layers/image:main", "github.com/org/repo", "layers/image", "image", "main"},
		{"@github.com/org/repo/layers/name", "github.com/org/repo", "layers/name", "name", ""},
		{"@github.com/org/repo/image", "github.com/org/repo", "image", "image", ""},
		{"pixi", "", "", "pixi", ""},
	}

	for _, tt := range tests {
		got := ParseRemoteRef(tt.ref)
		if got.RepoPath != tt.wantRepo || got.SubPath != tt.wantSub || got.Name != tt.wantName || got.Version != tt.wantVer {
			t.Errorf("ParseRemoteRef(%q) = {Repo: %q, SubPath: %q, Name: %q, Version: %q}, want {%q, %q, %q, %q}",
				tt.ref, got.RepoPath, got.SubPath, got.Name, got.Version, tt.wantRepo, tt.wantSub, tt.wantName, tt.wantVer)
		}
		if got.Raw != tt.ref {
			t.Errorf("ParseRemoteRef(%q).Raw = %q", tt.ref, got.Raw)
		}
	}
}

func TestIsRemoteImageRef(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"ollama", false},
		{"@github.com/org/repo/image", true},
		{"@github.com/org/repo/box:v1.0.0", true},
		{"github.com/org/repo/image", false}, // no @ prefix
	}

	for _, tt := range tests {
		got := IsRemoteImageRef(tt.ref)
		if got != tt.want {
			t.Errorf("IsRemoteImageRef(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

// TestParseRefKind pins the DISCRIMINATOR: which of the three arms a ref belongs to, in
// both spellings (a ref SITE carries `@`, a SOURCE field does not), and an actionable
// refusal for every scheme nobody supports. The refusal is the point — the reason this
// classifier exists is that `@s3://bucket/x` and `@oci:/x` used to reach the git-only
// ParseRemoteRef and come out as the repo path `oci`, far from the line that made the
// mistake.
func TestParseRefKind(t *testing.T) {
	tests := []struct {
		ref      string
		want     RefKind
		wantErr  bool
		errNames string // a substring the refusal must carry, when wantErr
	}{
		// Bare — a local candy name, no prefix.
		{ref: "pixi", want: RefKindBare},
		{ref: "layer-with-many-parts", want: RefKindBare},

		// Git — both spellings.
		{ref: "github.com/org/repo", want: RefKindGit},
		{ref: "github.com/org/repo/candy/x", want: RefKindGit},
		{ref: "@github.com/org/repo/candy/x:v1", want: RefKindGit},

		// Registry — both spellings.
		{ref: "oci://ghcr.io/org/repo", want: RefKindOci},
		{ref: "oci://ghcr.io/org/repo:v1", want: RefKindOci},
		{ref: "@oci://ghcr.io/org/repo:v1", want: RefKindOci},

		// Refusals: an unsupported scheme names the supported ones, never a bare
		// "invalid value" that leaves the author guessing.
		{ref: "@s3://bucket/prefix", wantErr: true, errNames: "oci://"},
		{ref: "@ftp://host/x", wantErr: true, errNames: "github.com/"},
		{ref: "@oci:/x", wantErr: true, errNames: "unsupported ref"},
		{ref: "@", wantErr: true, errNames: "unsupported ref"},
		{ref: "ghcr.io/org/repo", wantErr: true, errNames: "unsupported ref"},
		{ref: "Not-A-Candy-Name", wantErr: true, errNames: "unsupported ref"},
		{ref: "", wantErr: true, errNames: "empty ref"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := ParseRefKind(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRefKind(%q) = %v, want an error", tt.ref, got)
				}
				if !strings.Contains(err.Error(), tt.errNames) {
					t.Errorf("ParseRefKind(%q) error = %q, want it to name %q", tt.ref, err, tt.errNames)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRefKind(%q) unexpected error: %v", tt.ref, err)
			}
			if got != tt.want {
				t.Errorf("ParseRefKind(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

// TestParseOciRef pins the DECOMPOSITION the consumer keys its store by: registry,
// repository, tag, digest. The two shapes worth guarding are the registry PORT (which
// must never be read as a tag) and the DIGEST (which must never be silently accepted
// malformed — a digest the consumer cannot verify is worse than no digest, because it
// looks like an exact pin).
func TestParseOciRef(t *testing.T) {
	tests := []struct {
		ref            string
		wantRegistry   string
		wantRepository string
		wantTag        string
		wantDigest     string
		wantErr        bool
	}{
		{ref: "oci://ghcr.io/opencharly/plugin-bpf", wantRegistry: "ghcr.io", wantRepository: "opencharly/plugin-bpf"},
		{ref: "oci://ghcr.io/opencharly/plugin-bpf:2026.286.0739", wantRegistry: "ghcr.io", wantRepository: "opencharly/plugin-bpf", wantTag: "2026.286.0739"},
		{ref: "@oci://ghcr.io/opencharly/plugin-bpf:v1", wantRegistry: "ghcr.io", wantRepository: "opencharly/plugin-bpf", wantTag: "v1"},
		// A registry PORT is not a tag.
		{ref: "oci://127.0.0.1:5001/opencharly/plugin-bpf:v1", wantRegistry: "127.0.0.1:5001", wantRepository: "opencharly/plugin-bpf", wantTag: "v1"},
		{ref: "oci://127.0.0.1:5001/opencharly/plugin-bpf", wantRegistry: "127.0.0.1:5001", wantRepository: "opencharly/plugin-bpf"},
		{ref: "oci://ghcr.io/x/y@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
			wantRegistry: "ghcr.io", wantRepository: "x/y",
			wantDigest: "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"},
		{ref: "oci://ghcr.io/x/y:v1@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
			wantRegistry: "ghcr.io", wantRepository: "x/y", wantTag: "v1",
			wantDigest: "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"},

		{ref: "github.com/org/repo", wantErr: true},
		{ref: "@oci://", wantErr: true},
		{ref: "oci://ghcr.io", wantErr: true},
		{ref: "oci:///repo", wantErr: true},
		{ref: "oci://ghcr.io/x/y@sha256:deadbeef", wantErr: true},
		{ref: "oci://ghcr.io/x/y@md5:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := ParseOciRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseOciRef(%q) = %+v, want an error", tt.ref, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOciRef(%q) unexpected error: %v", tt.ref, err)
			}
			if got.Registry != tt.wantRegistry || got.Repository != tt.wantRepository ||
				got.Tag != tt.wantTag || got.Digest != tt.wantDigest {
				t.Errorf("ParseOciRef(%q) = {registry %q, repo %q, tag %q, digest %q}, want {%q, %q, %q, %q}",
					tt.ref, got.Registry, got.Repository, got.Tag, got.Digest,
					tt.wantRegistry, tt.wantRepository, tt.wantTag, tt.wantDigest)
			}
			if got.Raw != tt.ref {
				t.Errorf("ParseOciRef(%q).Raw = %q", tt.ref, got.Raw)
			}
		})
	}
}
