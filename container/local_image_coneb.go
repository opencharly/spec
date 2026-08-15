package container

// local_image_coneb.go — resolving a user-supplied image reference against local podman/docker
// storage, RELOCATED to the spec/container fabric slice (#55 coneB build-render cone, Class A —
// the resolution runs HOST-SIDE for overlay envelope prep, so host inlining from spec/container
// is the correct how: these are pure container-CLI probes + ref-shape predicates with no
// charly-core state). The body is verbatim from sdk/kit/local_image.go (#55 value extraction);
// sdk/kit re-exports every symbol (sdk/kit/local_image.go is now a thin re-export file) so the
// ~6 plugin consumers (candy/plugin-clean, candy/plugin-vm, candy/plugin-box, candy/plugin-build,
// candy/plugin-deploy-pod, candy/plugin-kube) and charly core's remaining callers keep their
// kit.X call sites unchanged. New consumers reference spec/container directly.
//
// Dependencies carried here are all already fabric: EngineBinary (spec/container), InspectImageLabels
// (spec/container, coneA), CompareCalVer (spec/spec), ErrImageNotLocal/LabelVersion/LabelBox
// (spec/spec). LocalImageExists (formerly sdk/kit/transfer.go) is co-located here because
// ResolveLocalImageRef reads it — a spec/container function cannot call back into kit (cycle), so
// the var's canonical home moves with the family; kit re-exports it.

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/opencharly/spec/spec"
)

// ErrImageNotLocal is re-exported by kit (kit.ErrImageNotLocal = spec.ErrImageNotLocal); the
// canonical sentinel lives in spec/spec/image_errors.go. Referenced here via spec.ErrImageNotLocal.

// LocalImageExists checks whether an image reference exists in the given engine's local store.
// Package-level var for testability (same pattern as DetectEngine). Co-located with the
// resolution family because ResolveLocalImageRef reads it; kit re-exports it
// (kit.LocalImageExists = container.LocalImageExists) so existing direct callers (candy/plugin-build,
// candy/plugin-box, candy/plugin-deploy-pod, candy/plugin-kube, charly core) are unchanged.
var LocalImageExists = defaultLocalImageExists

func defaultLocalImageExists(engine, imageRef string) bool {
	binary := EngineBinary(engine)
	switch engine {
	case "podman":
		cmd := exec.Command(binary, "image", "exists", imageRef)
		return cmd.Run() == nil
	default:
		// Docker has no "image exists" subcommand; use "image inspect"
		cmd := exec.Command(binary, "image", "inspect", imageRef)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Run() == nil
	}
}

// LooksLikeFullRef returns true if the image ref contains a registry segment
// (a "/" before any ":") — e.g. "ghcr.io/org/name:tag" — so it can be pulled
// without charly.yml resolution.
func LooksLikeFullRef(ref string) bool {
	if strings.HasPrefix(ref, "@") {
		return false
	}
	slash := strings.Index(ref, "/")
	if slash < 0 {
		return false
	}
	colon := strings.Index(ref, ":")
	return colon < 0 || slash < colon
}

// LocalImageInfo describes an image present in the engine's local storage.
// Populated by ListLocalImages from `{podman,docker} images --format json`.
type LocalImageInfo struct {
	ID     string            // image ID (sha256:...) — used by `charly clean` to skip in-use images
	Names  []string          // Full refs: ["ghcr.io/opencharly/jupyter:latest", ...]
	Labels map[string]string // OCI labels from the image config
	Size   int64             // reported storage size in bytes (podman's "Size" field; 0 if absent/unparsed)
}

// ListLocalImages returns all images in the engine's local storage.
// Package-level var for testability (same pattern as LocalImageExists, DetectEngine).
var ListLocalImages = defaultListLocalImages

func defaultListLocalImages(engine string) ([]LocalImageInfo, error) {
	binary := EngineBinary(engine)
	cmd := exec.Command(binary, "images", "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing local images via %s: %w", binary, err)
	}
	return ParseLocalImagesJSON(out)
}

// ParseLocalImagesJSON parses `{podman,docker} images --format json` output
// into ONE LocalImageInfo per distinct image ID, with that id's tag refs
// merged into Names.
//
// This dedup is load-bearing: podman emits ONE ROW PER TAG, and each row's
// Names array already lists EVERY tag on that id. A naive row-by-row mapping
// therefore produces N near-identical entries for an id with N tags — which
// made `charly clean`'s keep-N retention over-count entries and, worse, remove an
// id's whole Names array per "extra" entry (deleting tags it meant to keep).
// Collapsing to one-id-with-a-tag-list matches the struct's shape and what
// every consumer (retention prune, short-name resolver) expects.
//
// Empty-id rows (dangling/untagged) are kept separate via a per-row sentinel
// key so they never merge into one another.
func ParseLocalImagesJSON(out []byte) ([]LocalImageInfo, error) {
	var rawImages []map[string]any
	if err := json.Unmarshal(out, &rawImages); err != nil {
		return nil, fmt.Errorf("parsing images output: %w", err)
	}
	byKey := make(map[string]*LocalImageInfo)
	order := make([]string, 0, len(rawImages))
	for i, raw := range rawImages {
		// Image ID: podman uses "Id", docker uses "ID".
		id := ""
		if s, ok := raw["Id"].(string); ok {
			id = s
		} else if s, ok := raw["ID"].(string); ok {
			id = s
		}
		key := id
		if key == "" {
			key = fmt.Sprintf("\x00row%d", i) // never merge distinct untagged images
		}
		info, ok := byKey[key]
		if !ok {
			info = &LocalImageInfo{ID: id, Labels: make(map[string]string)}
			byKey[key] = info
			order = append(order, key)
		}
		// Tag refs: podman uses "Names", docker uses "RepoTags". Merge + dedup.
		var refs []string
		if names, ok := raw["Names"].([]any); ok {
			for _, n := range names {
				if s, ok := n.(string); ok {
					refs = append(refs, s)
				}
			}
		}
		if len(refs) == 0 {
			if tags, ok := raw["RepoTags"].([]any); ok {
				for _, t := range tags {
					if s, ok := t.(string); ok {
						refs = append(refs, s)
					}
				}
			}
		}
		seen := make(map[string]bool, len(info.Names))
		for _, n := range info.Names {
			seen[n] = true
		}
		for _, n := range refs {
			if !seen[n] {
				info.Names = append(info.Names, n)
				seen[n] = true
			}
		}
		// Labels are identical across rows for one id; first-writer wins.
		if labels, ok := raw["Labels"].(map[string]any); ok {
			for k, v := range labels {
				if s, ok := v.(string); ok {
					if _, exists := info.Labels[k]; !exists {
						info.Labels[k] = s
					}
				}
			}
		}
		// Size (bytes) is identical across rows for one id; podman JSON-decodes it as a
		// float64 (json.Unmarshal's numeric default into map[string]any). Absent/unparsed → 0.
		if sz, ok := raw["Size"].(float64); ok {
			info.Size = int64(sz)
		}
	}
	result := make([]LocalImageInfo, 0, len(order))
	for _, key := range order {
		result = append(result, *byKey[key])
	}
	return result, nil
}

// ResolveLocalImageRef resolves a user-supplied image reference against the
// engine's local storage — never reads charly.yml. Used by test-mode commands
// (charly check live, charly check box) so they stay within the test-mode input set.
//
// For full refs (registry prefix present) it validates the image exists
// locally and passes through unchanged. For short names it resolves via
// CalVer: collect every candidate ref and pick the one with the highest
// `ai.opencharly.version` label (falling back to the highest tag CalVer).
// charly is CalVer-only — no `:latest` fallback. See `/charly-build:build`
// "CalVer-only" for the contract.
//
// A candidate ref must satisfy BOTH halves: its image is identified as the short
// name (preferred: an `ai.opencharly.image=<short>` label; fallback: no such
// label) AND the ref ITSELF is named `<short>` in its trailing repo component.
// The second half is not cosmetic — a label-family image accumulates the deploy-name
// alias tags of every OTHER deployment built on the same base box, and they inherit
// its label, so without it an untagged resolve can return a sibling deployment's
// image (see the invariant comment in the label branch below).
//
// Returns `spec.ErrImageNotLocal` when nothing matches. An ambiguous result
// across multiple repos with the same highest CalVer tag surfaces as an
// explicit error asking for a full ref.
//
// This is the LENIENT form: it elects a ref and says nothing about what it passed over. A verb
// that pronounces a VERDICT on the artifact (`charly check box`, `charly box feature run`,
// `charly box labels`) must call ResolveBuiltImageRef instead, which refuses to elect an image
// older than the newest local build.
func ResolveLocalImageRef(engine, input string) (string, error) {
	res, err := ResolveLocalImage(engine, input)
	if err != nil {
		return "", err
	}
	return res.Ref, nil
}

// LocalImageResolution is the COMPLETE answer to "which local image does this reference name?":
// the ref the ordering elects (Ref) plus the newest local BUILD carrying the same short name
// (NewestBuildRef). The two diverge whenever the ordering's PRIMARY key — the content-derived
// `ai.opencharly.version` label — elects an image that is not the most recently built one, which
// is exactly the shape that makes a build-scope verdict certify the wrong artifact. The lenient
// ResolveLocalImageRef keeps only Ref; a verb that PRONOUNCES on an artifact resolves through
// ResolveBuiltImageRef instead, which refuses that divergence.
type LocalImageResolution struct {
	// Ref is the elected reference — what ResolveLocalImageRef returns.
	Ref string
	// NewestBuildRef is the candidate ref carrying the highest build-tag CalVer across BOTH
	// candidate families (label-identified AND name-identified), so a build whose
	// `ai.opencharly.box` label disagrees with its own repo name is still counted. Empty when no
	// candidate carries a CalVer build tag at all (every ref is a deploy alias or a float).
	NewestBuildRef string
	// Pinned reports that the input named a full ref or an explicit tag: the operator stated
	// which artifact they meant, so no election happened and nothing is ambiguous.
	Pinned bool
}

// ResolveLocalImage is ResolveLocalImageRef's full-answer form — same election, same errors, plus
// the newest-build ref the election may have passed over. Every caller that only needs the elected
// ref goes through ResolveLocalImageRef; the build-scope verdict verbs go through
// ResolveBuiltImageRef. Both are thin wrappers over this one body (R3 — one resolution, one
// candidate gather, one ordering).
func ResolveLocalImage(engine, input string) (LocalImageResolution, error) {
	if LooksLikeFullRef(input) {
		if !LocalImageExists(engine, input) {
			return LocalImageResolution{}, fmt.Errorf("%w: %s", spec.ErrImageNotLocal, input)
		}
		return LocalImageResolution{Ref: input, NewestBuildRef: input, Pinned: true}, nil
	}
	shortName, requestedTag := splitShortTaggedImage(input)

	images, err := ListLocalImages(engine)
	if err != nil {
		return LocalImageResolution{}, err
	}

	var labelCands, nameCands []resolverCandidate
	for _, img := range images {
		labelCalVer := img.Labels[spec.LabelVersion] // content-derived EffectiveVersion (primary key)
		// Label-preferred: ai.opencharly.image equals the short name.
		if img.Labels[spec.LabelBox] == shortName && shortName != "" {
			for _, n := range img.Names {
				if requestedTag != "" && !refHasExactTag(n, requestedTag) {
					continue
				}
				// THE NAMING INVARIANT: a candidate ref must itself be NAMED shortName.
				// A label-family image carries every tag ever put on it, including the
				// `<registry>/<deploy-name>:<calver>` aliases `tagDeployAlias` mints for
				// OTHER deployments of this same base box (they inherit the base's
				// ai.opencharly.image label, so they land in this branch). Those aliases are
				// resolvable by their OWN deploy name through the name-fallback below; they
				// are never the answer for the base box's name. Without this filter the
				// candidates all share one content-derived label-CalVer, so the tag-CalVer
				// tiebreak silently elected whichever SIBLING deployment happened to be
				// (re)deployed most recently — the cross-deployment image-crossing defect.
				if !shortNameMatchesRef(n, shortName) {
					continue
				}
				// label-CalVer is the PRIMARY ordering key; tag-CalVer (the
				// per-build timestamp) is the TIEBREAKER that picks the newest
				// BUILD among images sharing one content-stable label. No
				// label↔tag substitution — they are independent keys.
				labelCands = append(labelCands, resolverCandidate{
					ref:         n,
					labelCalVer: labelCalVer,
					tagCalVer:   ExtractCalVerTag(n),
				})
			}
			continue
		}
		// Name-fallback: any of the image's tags has the short name as
		// its trailing repo component. This catches `<deploy-name>:<calver>`
		// aliases (tagDeployAlias) on overlay images that inherited
		// the base image's label.
		for _, name := range img.Names {
			if requestedTag != "" && !refHasExactTag(name, requestedTag) {
				continue
			}
			if shortNameMatchesRef(name, shortName) {
				nameCands = append(nameCands, resolverCandidate{
					ref:         name,
					labelCalVer: labelCalVer,
					tagCalVer:   ExtractCalVerTag(name),
				})
			}
		}
	}

	cands := labelCands
	if len(cands) == 0 {
		cands = nameCands
	}
	if len(cands) == 0 {
		return LocalImageResolution{}, fmt.Errorf("%w: %s", spec.ErrImageNotLocal, input)
	}

	// Sort newest-first. The label-CalVer (the content-derived
	// ai.opencharly.version) is the PRIMARY key — it ALWAYS takes priority
	// over the tag-CalVer. The tag-CalVer (the per-build YYYY.DDD.HHMM
	// timestamp) is the TIEBREAKER: a content-stable label means many builds
	// share one label-CalVer, so the tag is what selects the newest BUILD.
	// YYYY.DDD.HHMM is NOT lexically sortable (DDD 1-366, HHMM 0-2359, both
	// variable-width) — compareCalVerKey parses each component numerically; an
	// empty CalVer sorts last (compareCalVerKey).
	// There is no trailing-segment tiebreak here any more: BOTH candidate sets are
	// now FILTERED on shortNameMatchesRef, so every survivor is named shortName and a
	// prefer-the-exact-name tiebreak could never fire. Preferring the base at sort time
	// was too weak anyway — it only broke exact CalVer ties, so a sibling deployment's
	// alias with a NEWER tag-CalVer won outright.
	sort.SliceStable(cands, func(i, j int) bool {
		// Primary: label-CalVer descending (label > tag, always).
		if c := compareCalVerKey(cands[i].labelCalVer, cands[j].labelCalVer); c != 0 {
			return c > 0
		}
		// Tiebreaker: tag-CalVer descending (newest build).
		if c := compareCalVerKey(cands[i].tagCalVer, cands[j].tagCalVer); c != 0 {
			return c > 0
		}
		return cands[i].ref < cands[j].ref
	})

	// If the top candidate has NEITHER a label-CalVer NOR a tag-CalVer AND
	// there are multiple distinct repositories among the candidates, that's a
	// genuine cross-repo ambiguity (e.g. two third-party `:latest` tags).
	// Surface the full list so the user can disambiguate with a full ref.
	if cands[0].labelCalVer == "" && cands[0].tagCalVer == "" && !sameRepoAcross(cands) {
		refs := make([]string, len(cands))
		for i, c := range cands {
			refs[i] = c.ref
		}
		return LocalImageResolution{}, fmt.Errorf("ambiguous short name %q in local storage; candidates: %s. Re-run with a full ref",
			input, strings.Join(refs, ", "))
	}

	// The newest-BUILD probe spans BOTH families, not just the elected one. The families split on
	// the `ai.opencharly.box` label, and a build whose label disagrees with its own repo name
	// (the namespaced-label defect this cutover fixes at the emitter) lands in the OTHER family —
	// which the election discards wholesale. Spanning both is what makes the staleness visible on
	// storage that still holds such images.
	all := make([]resolverCandidate, 0, len(labelCands)+len(nameCands))
	all = append(all, labelCands...)
	all = append(all, nameCands...)
	return LocalImageResolution{
		Ref:            cands[0].ref,
		NewestBuildRef: newestBuildRef(all),
		Pinned:         requestedTag != "",
	}, nil
}

// newestBuildRef returns the candidate ref with the highest build-tag CalVer, or "" when no
// candidate carries one. The build tag is the per-build `YYYY.DDD.HHMM` timestamp, so this is
// literally "the most recently built artifact among these" — independent of the content-derived
// label CalVer the election orders by.
func newestBuildRef(cands []resolverCandidate) string {
	best := ""
	bestCalVer := ""
	for _, c := range cands {
		if c.tagCalVer == "" {
			continue
		}
		if best == "" || compareCalVerKey(c.tagCalVer, bestCalVer) > 0 {
			best, bestCalVer = c.ref, c.tagCalVer
		}
	}
	return best
}

// ResolveBuiltImageRef resolves like ResolveLocalImageRef and then REFUSES the resolution when the
// elected image is not the newest local BUILD of that short name.
//
// It is the resolver for every verb that pronounces a VERDICT on a built artifact — `charly check
// box`, `charly box feature run`, `charly box labels`. Those verbs answer "what is in the image I
// just built", and the election cannot answer that on its own: its PRIMARY key is the
// content-derived `ai.opencharly.version` label, so an image built from a differently-versioned
// source tree outranks a newer build regardless of when either was produced. A green verdict
// against the older artifact is the worst failure this system has — it looks exactly like a
// passing one — so the ambiguity is surfaced as an error and the operator names the artifact.
//
// A full ref or an explicit `<name>:<tag>` states the intent and passes through untouched, which
// is why the R10 bed sequence (which builds and then checks `<image>:<run-tag>`) never reaches
// this guard.
func ResolveBuiltImageRef(engine, input string) (string, error) {
	res, err := ResolveLocalImage(engine, input)
	if err != nil {
		return "", err
	}
	if err := res.RefuseIfStale(input); err != nil {
		return "", err
	}
	return res.Ref, nil
}

// RefuseIfStale returns a `spec.ErrStaleLocalImage`-wrapped error when the elected ref is older
// (by build tag) than the newest local build of the same short name. Pinned resolutions never
// refuse: the operator already said which artifact they meant.
func (r LocalImageResolution) RefuseIfStale(input string) error {
	if r.Pinned || r.NewestBuildRef == "" || r.NewestBuildRef == r.Ref {
		return nil
	}
	if compareCalVerKey(ExtractCalVerTag(r.NewestBuildRef), ExtractCalVerTag(r.Ref)) <= 0 {
		return nil
	}
	return fmt.Errorf("%w: %q resolves to %s, but %s is a newer local build of the same box. "+
		"A build-scope verdict on the older artifact would certify the wrong image, so charly refuses to choose for you. "+
		"Re-run naming the artifact you mean — `%s:%s` for the newest build, or any full ref",
		spec.ErrStaleLocalImage, input, r.Ref, r.NewestBuildRef,
		refRepoName(r.NewestBuildRef), ExtractCalVerTag(r.NewestBuildRef))
}

// splitShortTaggedImage separates the standard registry-less `name:tag` form.
// Registry-qualified references are handled before this helper, so a colon here
// cannot be a registry port. Keeping the requested tag separate lets the local
// resolver match the full stored ref while preserving label-based short-name
// lookup.
func splitShortTaggedImage(input string) (name, tag string) {
	i := strings.LastIndex(input, ":")
	if i <= 0 || i == len(input)-1 {
		return input, ""
	}
	return input[:i], input[i+1:]
}

func refHasExactTag(ref, tag string) bool {
	i := strings.LastIndex(ref, ":")
	return i > strings.LastIndex(ref, "/") && i < len(ref)-1 && ref[i+1:] == tag
}

// resolverCandidate pairs a full image ref with its two CalVer keys: the
// labelCalVer (ai.opencharly.version — the content-derived EffectiveVersion,
// the PRIMARY ordering key) and the tagCalVer (the `:<calver>` build-timestamp
// tag, the TIEBREAKER). Used internally by ResolveLocalImageRef to sort
// candidates newest-first before picking one.
type resolverCandidate struct {
	ref         string
	labelCalVer string
	tagCalVer   string
}

// compareCalVerKey orders two CalVer strings with "" sorting LAST (lowest
// rank): returns >0 when a ranks higher (newer) than b, <0 when lower, 0 when
// equal. A non-empty CalVer always outranks an empty one.
func compareCalVerKey(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	return spec.CompareCalVer(a, b)
}

// sameRepoAcross reports whether every candidate ref shares the same
// repository path (everything before the final `:<tag>`). Used to
// distinguish benign duplicate-tag cases (one image, multiple tags)
// from genuinely ambiguous matches (same short name across multiple
// unrelated repos).
func sameRepoAcross(cands []resolverCandidate) bool {
	if len(cands) <= 1 {
		return true
	}
	repoOf := func(ref string) string {
		if lastSlash := strings.LastIndex(ref, "/"); lastSlash >= 0 {
			if colon := strings.LastIndex(ref, ":"); colon > lastSlash {
				return ref[:colon]
			}
		} else if colon := strings.LastIndex(ref, ":"); colon >= 0 {
			return ref[:colon]
		}
		return ref
	}
	first := repoOf(cands[0].ref)
	for _, c := range cands[1:] {
		if repoOf(c.ref) != first {
			return false
		}
	}
	return true
}

// ExtractCalVerTag returns the CalVer portion of a ref's tag, or ""
// if the tag is not a recognisable CalVer (`YYYY.DDD.HHMM`). Lets the
// resolver distinguish CalVer tags from legacy floats like `:latest`
// (which should never be chosen as the newest candidate).
func ExtractCalVerTag(ref string) string {
	// Find the tag portion: last ':' after the last '/'.
	tagStart := -1
	if lastSlash := strings.LastIndex(ref, "/"); lastSlash >= 0 {
		if colon := strings.LastIndex(ref, ":"); colon > lastSlash {
			tagStart = colon + 1
		}
	} else if colon := strings.LastIndex(ref, ":"); colon >= 0 {
		tagStart = colon + 1
	}
	if tagStart < 0 || tagStart >= len(ref) {
		return ""
	}
	tag := ref[tagStart:]
	// CalVer shape: three dot-separated decimal parts. Legacy
	// `:latest` / `:stable` / `:dev` floats fall through.
	parts := strings.Split(tag, ".")
	if len(parts) != 3 {
		return ""
	}
	for _, p := range parts {
		if p == "" {
			return ""
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return ""
			}
		}
	}
	return tag
}

// ResolveNewestLocalCalVer is the canonical "find the newest local
// image for this short name" helper. Thin wrapper around
// ResolveLocalImageRef — exposed so callers that start with an
// explicit short-name + empty-tag can resolve uniformly.
func ResolveNewestLocalCalVer(engine, short string) (string, error) {
	return ResolveLocalImageRef(engine, short)
}

// ResolveShellImageRef builds the full image reference from registry, name, and tag, for a
// caller about to run an engine command (podman/docker) against it. When tag is empty, it
// resolves to the newest local CalVer for the given short name via ResolveNewestLocalCalVer —
// the CalVer-only contract (`/charly-build:build` "Cache Efficiency"). A caller that wants a
// specific tag passes it; a caller whose `--tag` flag is empty gets the newest CalVer with no
// extra work. When registry is set AND tag is empty, there's no way to guess a remote CalVer
// without a registry-list call, so the caller gets `<registry>/<name>` back with no tag suffix —
// the engine resolves it locally first (matching any single local tag) or errors.
// (P14: relocated from charly/shell.go's resolveShellImageRef, which now delegates here — R3,
// single source for every caller: candy/plugin-box's `merge` command plus charly core's own
// fleet_add/config_image/ensure_image/remote_image/pod_lifecycle_resolve/update_deploy_dispatch.)
func ResolveShellImageRef(registry, name, tag string) string {
	if tag == "" {
		// Try local CalVer resolution. Best-effort: if nothing local matches, fall back to a
		// tagless ref so the engine's own resolution path can error with its canonical message.
		if resolved, err := ResolveNewestLocalCalVer("podman", name); err == nil && resolved != "" {
			return resolved
		}
		if registry != "" {
			return fmt.Sprintf("%s/%s", registry, name)
		}
		return name
	}
	if registry != "" {
		return fmt.Sprintf("%s/%s:%s", registry, name, tag)
	}
	return fmt.Sprintf("%s:%s", name, tag)
}

// shortNameMatchesRef reports whether a short name like "jupyter" matches a
// full ref like "ghcr.io/opencharly/jupyter:latest" by comparing the trailing
// repo component (after the last "/", before the tag).
//
// This is the SOLE trailing-segment predicate — ResolveLocalImageRef filters BOTH its
// label-family and its name-fallback candidate sets through it (a second, subtly
// different inline copy used to exist as the sort tiebreak; it is deleted).
func shortNameMatchesRef(fullRef, short string) bool {
	return refRepoName(fullRef) == short
}

// refRepoName returns a ref's trailing repository segment — the name a short-name
// resolve has to match. It strips a digest, then a tag (the last ":" AFTER the last
// "/", so a `host:port/repo` registry port is never mistaken for a tag), then takes
// the segment after the final "/".
func refRepoName(fullRef string) string {
	repo := fullRef
	if at := strings.Index(repo, "@"); at >= 0 {
		repo = repo[:at]
	}
	lastSlash := strings.LastIndex(repo, "/")
	if colon := strings.LastIndex(repo, ":"); colon > lastSlash {
		repo = repo[:colon]
	}
	return repo[lastSlash+1:]
}
