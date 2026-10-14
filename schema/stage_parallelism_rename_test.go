package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// boxDefaultsDoc compiles the shipped schema and unifies #Box (the authored
// defaults/box shape) with the given document, returning the error (nil when the
// document satisfies the def). Same concat the runtime and `charly task cue-gen`
// use, so this exercises the schema as it ships.
func boxDefaultsDoc(t *testing.T, doc string) error {
	t.Helper()
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	def := v.LookupPath(cue.ParsePath("#Box"))
	if !def.Exists() {
		t.Fatal("#Box is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(doc))
	if err := unified.Err(); err != nil {
		return err
	}
	return unified.Validate(cue.Concrete(false))
}

// POSITIVE CONTROL — it must run and pass, or the negative arm below would pass
// vacuously (a def that failed to load would reject everything).
func TestBoxAcceptsStageParallelism(t *testing.T) {
	if err := boxDefaultsDoc(t, `{stage_parallelism: 4, stage_parallelism_cap: 8}`); err != nil {
		t.Fatalf("the renamed parallelism knob was rejected: %v", err)
	}
	// The OUTER knob is untouched and keeps its own name: `jobs` bounds images per
	// DAG level, `stage_parallelism` bounds the stages of ONE build.
	if err := boxDefaultsDoc(t, `{jobs: 4}`); err != nil {
		t.Fatalf("the outer jobs knob was rejected: %v", err)
	}
}

// The retired name is rejected by the closed def, so an authored config that still
// carries it fails loudly and points the reader at `charly migrate` (whose table
// entry renames it — the entry lands in plugin-migrate, in the leg that follows).
//
// This test FAILS without the rename: while `podman_jobs` was declared, this
// document unified cleanly.
func TestBoxRejectsRetiredPodmanJobs(t *testing.T) {
	if err := boxDefaultsDoc(t, `{podman_jobs: 4}`); err == nil {
		t.Fatal("the retired podman_jobs key must be rejected by the closed #Box def")
	}
	if err := boxDefaultsDoc(t, `{podman_jobs_cap: 8}`); err == nil {
		t.Fatal("the retired podman_jobs_cap key must be rejected by the closed #Box def")
	}
}
