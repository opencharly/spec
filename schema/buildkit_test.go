package schema_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
	specpkg "github.com/opencharly/spec/spec"
)

// buildkitDoc compiles the shipped schema and returns the load-time document gate
// (#NodeDoc) unified with the given charly.yml document, plus the unification error.
//
// It is the SAME concatenation the runtime loader and `charly task cue-gen` use, so
// a document accepted here is accepted by the gate that actually guards a project —
// not by a hand-built copy of the schema.
func buildkitDoc(t *testing.T, doc string) (cue.Value, error) {
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
	def := v.LookupPath(cue.ParsePath("#NodeDoc"))
	if !def.Exists() {
		t.Fatal("#NodeDoc is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(doc))
	if err := unified.Err(); err != nil {
		return unified, err
	}
	return unified, unified.Validate(cue.Concrete(false))
}

// TestBuildkitDefaultsAcceptedAndDecoded is the POSITIVE CONTROL, and it runs first.
//
// Without it a gate that failed to find `defaults.buildkit` at all would unify with
// anything, and every negative arm below would pass vacuously. The decoded values
// prove the authored document reaches the generated `Buildkit` struct that the build
// drive renders into buildkitd.toml — the whole point of the shape.
func TestBuildkitDefaultsAcceptedAndDecoded(t *testing.T) {
	doc := `
defaults: buildkit: {
	max_parallelism: 4
	process_mode: "sandbox"
	snapshotter: "overlayfs"
	no_process_sandbox: true
	gc: true
	reserved_space: "40GB"
	max_used_space: "80GB"
	min_free_space: "5GB"
	registry_mirror: "localhost:5050"
	gcpolicy: [{
		max_used_space: "60GB"
		keep_duration: "168h"
		all: true
		filters: ["type==source.local"]
	}]
	extra: "[registry]\n  mirrors = []\n"
}
`
	unified, err := buildkitDoc(t, doc)
	if err != nil {
		t.Fatalf("a legitimate defaults.buildkit was rejected: %v", err)
	}
	var got specpkg.Buildkit
	if err := unified.LookupPath(cue.ParsePath("defaults.buildkit")).Decode(&got); err != nil {
		t.Fatalf("decoding defaults.buildkit into spec.Buildkit: %v", err)
	}
	if got.MaxParallelism != 4 || got.ReservedSpace != "40GB" || got.MaxUsedSpace != "80GB" ||
		got.MinFreeSpace != "5GB" || got.RegistryMirror != "localhost:5050" ||
		!got.NoProcessSandbox || got.GC == nil || !*got.GC {
		t.Fatalf("the decoded Buildkit does not carry the authored values: %+v", got)
	}
	if len(got.GCPolicy) != 1 || got.GCPolicy[0].MaxUsedSpace != "60GB" ||
		got.GCPolicy[0].KeepDuration != "168h" || !got.GCPolicy[0].All ||
		len(got.GCPolicy[0].Filters) != 1 {
		t.Fatalf("the decoded gcpolicy does not carry the authored entry: %+v", got.GCPolicy)
	}
	if !strings.Contains(got.Extra, "mirrors = []") {
		t.Fatalf("the raw extra TOML did not survive the decode: %q", got.Extra)
	}

	// An empty block is legal: every knob is optional and selects the daemon's own
	// default (except the GC budget, which the drive always writes).
	if _, err := buildkitDoc(t, `defaults: buildkit: {}`); err != nil {
		t.Fatalf("an empty defaults.buildkit was rejected: %v", err)
	}

	// The wire half: the resolve reply carries the same shape to the drive.
	reply := `
{engine: "podman", buildkit: {max_parallelism: 2, reserved_space: "40GB"}}
`
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(schemaSrc)
	def := v.LookupPath(cue.ParsePath("#BuildResolveReply"))
	if !def.Exists() {
		t.Fatal("#BuildResolveReply is not defined in the shipped schema")
	}
	u := def.Unify(ctx.CompileString(reply))
	if err := u.Err(); err != nil {
		t.Fatalf("a resolve reply carrying buildkit was rejected: %v", err)
	}
	var r specpkg.BuildResolveReply
	if err := u.Decode(&r); err != nil {
		t.Fatalf("decoding the resolve reply: %v", err)
	}
	if r.Buildkit == nil || r.Buildkit.MaxParallelism != 2 {
		t.Fatalf("the resolve reply did not carry buildkit: %+v", r.Buildkit)
	}
}

// TestBuildkitDefaultsRejectsNegativeParallelism — the range constraint. This arm
// FAILS on the unmodified tree: `defaults` was an untyped directive, so any value at
// all (including a negative parallelism) unified cleanly and reached the Go decode.
func TestBuildkitDefaultsRejectsNegativeParallelism(t *testing.T) {
	if _, err := buildkitDoc(t, `defaults: buildkit: {max_parallelism: -1}`); err == nil {
		t.Fatal("a negative max_parallelism must be rejected by #Buildkit (int & >=0)")
	}
}

// TestBuildkitDefaultsRejectsUnknownKnob — #Buildkit is CLOSED, so a mistyped or
// retired daemon setting is a load error, never a value silently dropped by the drive.
func TestBuildkitDefaultsRejectsUnknownKnob(t *testing.T) {
	if _, err := buildkitDoc(t, `defaults: buildkit: {max_paralellism: 4}`); err == nil {
		t.Fatal("an unknown buildkit knob must be rejected by the closed #Buildkit def")
	}
}
