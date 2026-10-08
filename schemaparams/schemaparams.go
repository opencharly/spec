// Package schemaparams is THE schema→Go params pipeline (R3).
//
// One function runs the whole thing — concatenate a schema directory's `*.cue`
// files under a `package <pkg>` + `@go(<pkg>)` header, run the pinned
// `cue exp gengotypes`, then normalize the struct tags — so that the three places
// that need it cannot drift:
//
//   - spec's own generator (its `cue-gen` task) produces `spec/cue_types_gen.go`;
//   - the `charly`-side verb (`charly candy params`) produces a plugin candy's
//     `params/cue_types_gen.go`, and is a THIN wrapper over this;
//   - a plugin repo's CI can run it too, because this module is `go install`-able
//     (its tags are `v0.<YYYYDDD>.<HHMM>`) where the charly module's nested CalVer
//     tags are not — so a CI job gets the canonical pipeline without a charly
//     binary and without keeping its own copy of the recipe.
//
// The steps' contracts stay where they are, and this package only sequences them:
// `schemaconcat` (the concatenation), `cuetoolchain` (the pin and its fetcher),
// `schemaretag` (the tag normalization). The `gengotypes` generator itself lives in
// `cuelang.org/go`'s INTERNAL tree, which is why the pinned CLI is provisioned and
// run here rather than called in-process.
package schemaparams

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencharly/spec/cuetoolchain"
	"github.com/opencharly/spec/schemaconcat"
	"github.com/opencharly/spec/schemaretag"
)

// Generate provisions the pinned `cue` CLI into cueDir and returns the Go file the
// schema directory produces. cueDir is the caller's choice (spec's task uses `./bin`,
// the charly-side verb its own cache): the pin, the checksum, the verification and the
// arch rules all come from cuetoolchain, so every caller runs the SAME toolchain.
func Generate(schemaDir, pkg, cueDir string) ([]byte, error) {
	return GenerateExcluding(schemaDir, pkg, cueDir, nil)
}

// GenerateExcluding is Generate with a file filter: a schema whose disjunction
// wrappers (spec's `node.cue`) gengotypes must not see passes an `exclude` that names
// them. nil includes every `*.cue` file, which is what a plugin candy wants.
func GenerateExcluding(schemaDir, pkg, cueDir string, exclude func(name string) bool) ([]byte, error) {
	cueBin, err := cuetoolchain.Ensure(cueDir)
	if err != nil {
		return nil, err
	}
	return GenerateWithCueExcluding(schemaDir, pkg, cueBin, exclude)
}

// GenerateWithCue runs the pipeline with an explicit `cue` binary — the seam for a
// caller that has already provisioned one, and for a test that stands in for the
// toolchain rather than downloading it.
func GenerateWithCue(schemaDir, pkg, cueBin string) ([]byte, error) {
	return GenerateWithCueExcluding(schemaDir, pkg, cueBin, nil)
}

// GenerateWithCueExcluding is GenerateWithCue with a file filter (see
// GenerateExcluding).
func GenerateWithCueExcluding(schemaDir, pkg, cueBin string, exclude func(name string) bool) ([]byte, error) {
	body, files, err := schemaconcat.ConcatSchema(os.DirFS(schemaDir), ".", exclude)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", schemaDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no *.cue files in %s", schemaDir)
	}
	tmp, err := os.MkdirTemp("", "schemaparams-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// The header is part of the contract: `package <pkg>` names the Go package and
	// the file-level @go(<pkg>) attribute is what gengotypes reads to name it.
	src := "package " + pkg + "\n\n@go(" + pkg + ")\n\n" + body
	if err := os.WriteFile(filepath.Join(tmp, pkg+".cue"), []byte(src), 0o644); err != nil {
		return nil, err
	}
	// The generator runs with the TEMP dir as its working directory, so the binary
	// path must be absolute first: a caller that passes a relative one (spec's own
	// task passes `bin`, i.e. ./bin/cue) would otherwise fail with
	// `fork/exec bin/cue: no such file or directory`.
	absCue := cueBin
	if !filepath.IsAbs(absCue) {
		var aerr error
		if absCue, aerr = filepath.Abs(cueBin); aerr != nil {
			return nil, fmt.Errorf("resolving the toolchain path %q: %w", cueBin, aerr)
		}
	}
	cmd := exec.Command(absCue, "exp", "gengotypes", ".")
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("cue exp gengotypes failed in %s: %w\n%s", tmp, err, strings.TrimSpace(string(out)))
	}
	gen, err := os.ReadFile(filepath.Join(tmp, "cue_types_"+pkg+"_gen.go"))
	if err != nil {
		return nil, fmt.Errorf("gengotypes produced no cue_types_%s_gen.go: %w", pkg, err)
	}
	return schemaretag.Normalize(gen), nil
}
