package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// ref_parse.go — the generic remote-ref VOCAB: a `@host/org/repo/sub/path:version` ref's shape,
// and the pure string parsing that recognizes/decomposes it. Lives in spec (not a mechanism kit)
// because its consumer set is wide and NOT loader-cone-specific — deploy/provider/command files
// that merely need to recognize or decompose a ref string, with zero registry/host coupling. The
// LOADER-MECHANISM consumers of this vocab (repo-identity cycle-break, --repo spec normalization)
// live in sdk/loaderkit and import this package, never the reverse.

// DefaultProjectRepo is the repo --repo defaults to when the spec is the literal string "default"
// (or when charly mcp serve auto-falls back).
const DefaultProjectRepo = "github.com/opencharly/charly"

// The two supported remote SOURCE prefixes — the program-side spelling of the CUE
// arms `#GithubRef` / `#OciRef` (schema/candy.cue). ONE home: ParseRefKind, ParseOciRef
// and the unsupported-scheme messages all read them here, so a new arm is added once.
const (
	// GitSourcePrefix — a git module: fetched via the @github resolver and host-built
	// into a provider binary.
	GitSourcePrefix = "github.com/"
	// OciSourcePrefix — a registry artifact: fetched prebuilt for the consumer's
	// platform, so a consumer with no source checkout and no Go toolchain is served.
	OciSourcePrefix = "oci://"
)

// RefKind classifies a ref by WHAT MUST BE FETCHED — the program-side twin of the CUE
// ref arms (#CandyRef in schema/_common.cue, #PluginSource in schema/candy.cue).
//
// The CUE pattern is the gate for an AUTHORED document: it rejects an unsupported scheme
// at its file:line, quoting the pattern. This classifier is for a ref that arrives as a
// bare STRING — a CLI argument, a resolved envelope, a task input — where CUE cannot
// produce the message. It exists because the two vocabularies drifted once already:
// `#CandyRef`'s former `@.+` catch-all accepted `@oci:/x` and `@s3://bucket/x` in an
// authored doc, and the git-only ParseRemoteRef below then turned `@oci://host/repo`
// into the nonsense repo path `oci` — so the mistake surfaced far from the line that
// made it. Parse a ref with this FIRST; only a RefKindGit ref may go to ParseRemoteRef.
type RefKind string

const (
	// RefKindBare — a local candy name, resolved inside the project's own closure.
	RefKindBare RefKind = "bare"
	// RefKindGit — a git module (#GithubRef).
	RefKindGit RefKind = "git"
	// RefKindOci — a registry artifact (#OciRef).
	RefKindOci RefKind = "oci"
)

// bareCandyNameRe is the bare-name arm of every candy ref site (schema/_common.cue's
// `#CandyRef` third arm and `#EntityRef`): a lowercase-hyphenated name.
var bareCandyNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ociDigestRe is the `@sha256:<64 hex>` arm of #OciRef: the digest, when present, that
// the consumer verifies the fetched bytes against and keys its store by (D-STORE-3/4).
var ociDigestRe = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// ParseRefKind classifies ref, and reports an unsupported scheme with a message naming
// the supported arms and the value that failed.
//
// BOTH spellings are accepted: a ref SITE carries the `@` (require:/candy:/bake_plugin:),
// a SOURCE field does not (plugin.source:). One classifier for both, because that pairing
// is exactly what the CUE test (TestRefGrammarsAgreeWithTheirSourceShapes) pins — a caller
// never has to know which site it holds.
//
// Classification is by SCHEME PREFIX, deliberately: the full shape of an authored ref is
// the CUE pattern's job, and re-deriving it here would be a second copy to keep in sync
// (R2). What this rejects is the case CUE could not reach at all — a scheme nobody
// supports, handed over as a string.
func ParseRefKind(ref string) (RefKind, error) {
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}
	body := strings.TrimPrefix(ref, "@")
	switch {
	case strings.HasPrefix(body, OciSourcePrefix):
		return RefKindOci, nil
	case strings.HasPrefix(body, GitSourcePrefix):
		return RefKindGit, nil
	}
	if strings.HasPrefix(ref, "@") {
		return "", fmt.Errorf("unsupported ref %q: the @-prefixed arms are @%s… (git) and @%s… (registry); a bare local candy name carries no prefix", ref, GitSourcePrefix, OciSourcePrefix)
	}
	if bareCandyNameRe.MatchString(ref) {
		return RefKindBare, nil
	}
	return "", fmt.Errorf("unsupported ref %q: want a bare lowercase-hyphenated candy name, %s… (git) or %s… (registry)", ref, GitSourcePrefix, OciSourcePrefix)
}

// ParsedOciRef is a decomposed registry SOURCE ref:
// `[oci://]<registry>[:<port>]/<repository>[:<tag>][@sha256:<digest>]`.
type ParsedOciRef struct {
	Raw        string // original string, e.g. "oci://ghcr.io/opencharly/plugin-bpf:v1"
	Registry   string // e.g. "ghcr.io" (host, with :port when one was given)
	Repository string // e.g. "opencharly/plugin-bpf"
	Tag        string // e.g. "v1"          — empty when none was given
	Digest     string // e.g. "sha256:…"    — empty when none was given
}

// ParseOciRef decomposes an #OciRef-shaped source ref (either spelling — the `@` is
// accepted so a ref site and a source field parse identically).
//
// A tag is only recognized AFTER the last `/`, so a registry port (`:5000/…`) is never
// mistaken for one. The digest, when present, is validated as `sha256:<64 hex>`: a
// malformed digest is an error here rather than a digest the consumer would silently
// fail to verify later.
func ParseOciRef(ref string) (*ParsedOciRef, error) {
	raw := ref
	body := strings.TrimPrefix(ref, "@")
	if !strings.HasPrefix(body, OciSourcePrefix) {
		return nil, fmt.Errorf("not a registry ref %q: want the %s… scheme", raw, OciSourcePrefix)
	}
	rest := strings.TrimPrefix(body, OciSourcePrefix)

	out := &ParsedOciRef{Raw: raw}

	// Digest last (it is the last `@` component), then the tag (the last `:` after the
	// last `/`), then registry/repository.
	if idx := strings.LastIndex(rest, "@"); idx != -1 {
		out.Digest = rest[idx+1:]
		rest = rest[:idx]
		if !ociDigestRe.MatchString(out.Digest) {
			return nil, fmt.Errorf("invalid digest in ref %q: %q is not sha256:<64 hex>", raw, out.Digest)
		}
	}
	if slash := strings.LastIndex(rest, "/"); slash != -1 {
		if colon := strings.LastIndex(rest, ":"); colon > slash {
			out.Tag = rest[colon+1:]
			rest = rest[:colon]
		}
	}
	registry, repository, ok := strings.Cut(rest, "/")
	if !ok || registry == "" || repository == "" {
		return nil, fmt.Errorf("invalid registry ref %q: want %s<registry>[:<port>]/<repository>", raw, OciSourcePrefix)
	}
	out.Registry = registry
	out.Repository = repository
	return out, nil
}

// ParsedRef is a parsed remote reference: `@host/org/repo/sub/path:version`. Works for both candy
// refs and image refs.
type ParsedRef struct {
	Raw      string // original string, e.g. "@github.com/org/repo/candy/name:v1.0.0"
	RepoPath string // e.g. "github.com/org/repo"
	SubPath  string // e.g. "candy/name" (path within repo)
	Name     string // e.g. "name" (last segment)
	Version  string // e.g. "v1.0.0"
}

// IsRemoteImageRef returns true if a ref looks like a remote image reference (starts with @).
func IsRemoteImageRef(ref string) bool {
	return strings.HasPrefix(ref, "@")
}

// ParseRemoteRef parses a GIT remote reference into repo path, sub-path, name, and version.
// e.g. "@github.com/org/repo/candy/name:v1.0.0" -> ParsedRef{RepoPath: "github.com/org/repo", SubPath: "candy/name", Name: "name", Version: "v1.0.0"}
//
// It is GIT-ONLY, and it does not check: handed an `@oci://…` ref it splits on the last
// `:` and returns `oci` as the repo path — a plausible-looking wrong answer, which is
// exactly how the old `@.+` ref arm produced a nonsense git URL far from the authored
// line. A caller holding a ref of UNKNOWN kind must classify it first with ParseRefKind
// and route a RefKindOci ref to ParseOciRef; this function's contract is the git arm.
func ParseRemoteRef(ref string) *ParsedRef {
	raw := ref

	// Strip @ prefix
	ref = strings.TrimPrefix(ref, "@")

	// Split version
	version := ""
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		version = ref[idx+1:]
		ref = ref[:idx]
	}

	// Split into repo path (first 3 segments) and sub-path (rest)
	repoPath, subPath, name := SplitRepoAndSubPath(ref)

	return &ParsedRef{
		Raw:      raw,
		RepoPath: repoPath,
		SubPath:  subPath,
		Name:     name,
		Version:  version,
	}
}

// SplitRepoAndSubPath splits a ref into repo path (host/org/repo), sub-path, and name.
// e.g. "github.com/org/repo/candy/name" -> ("github.com/org/repo", "candy/name", "name")
// For short refs like "pixi", returns ("", "", "pixi").
func SplitRepoAndSubPath(ref string) (repoPath, subPath, name string) {
	parts := strings.SplitN(ref, "/", 4) // [host, org, repo, sub/path]
	if len(parts) < 4 {
		// Not enough segments for a remote ref — treat as local name
		name = parts[len(parts)-1]
		if len(parts) <= 1 {
			return "", "", name
		}
		return strings.Join(parts, "/"), "", name
	}
	repoPath = strings.Join(parts[:3], "/")
	subPath = parts[3]
	if idx := strings.LastIndex(subPath, "/"); idx != -1 {
		name = subPath[idx+1:]
	} else {
		name = subPath
	}
	return repoPath, subPath, name
}
