package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// deployShape compiles the shipped schema and unifies #Deploy with the given
// authored body, returning whether it satisfies the schema. The body is a bare
// literal (not prefixed) so a test can spell the whole deploy node.
func deployShape(t *testing.T, body string) error {
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
	def := v.LookupPath(cue.ParsePath("#Deploy"))
	if !def.Exists() {
		t.Fatal("#Deploy is not defined in the shipped schema")
	}
	unified := def.Unify(ctx.CompileString(body))
	if unified.Err() != nil {
		return unified.Err()
	}
	return unified.Validate(cue.Concrete(false))
}

// TestDeployShapeOverrideAcceptsCpuSingular gates the per-deploy VM-shape
// override's CPU spelling: a `from:` deploy body carries `cpu:` (singular),
// matching #Vm (the template it overrides).
//
// This test FAILS on the pre-cutover schema, where the field was misspelled
// `cpus:` — the exact regression the rename exists to prevent. Deleting the
// `cpu?:` field from #Deploy (or reverting it to `cpus:`) fails here.
func TestDeployShapeOverrideAcceptsCpuSingular(t *testing.T) {
	if err := deployShape(t, `{from: "some-vm", cpu: 2, ram: "4G"}`); err != nil {
		t.Fatalf("a from: deploy with `cpu:`/`ram:` was rejected: %v", err)
	}
}

// TestDeployShapeOverrideRejectsPluralCpus pins the cutover: the old `cpus:`
// spelling is GONE from the #Deploy arm. It survives only on #Security (a
// CPU-quota string) and the libvirt #LibvirtCPU.cpus — different defs entirely.
// If a future edit reintroduces `cpus:` as an accepted #Deploy key, this fails.
func TestDeployShapeOverrideRejectsPluralCpus(t *testing.T) {
	err := deployShape(t, `{from: "some-vm", cpus: 2}`)
	if err == nil {
		t.Fatal("the retired plural `cpus:` was accepted on a #Deploy body; the cutover is incomplete")
	}
}

// TestDeploySecurityCpusStillLive is the companion guard: the deploy body's
// nested `security:` block keeps its OWN `cpus:` (a CPU quota string on
// #Security — a different field/type). The #Deploy rename must not have swept
// it. This is the schema-side mirror of the migration's security-collision test.
func TestDeploySecurityCpusStillLive(t *testing.T) {
	if err := deployShape(t, `{from: "some-vm", security: {cpus: "2.5"}}`); err != nil {
		t.Fatalf("security.cpus (a #Security quota) was rejected — the rename over-swept: %v", err)
	}
}

// TestDeployRejectsVariants pins the deletion of the never-implemented
// `variants:`/`#VmVariant` surface: it was added in #86 with no reader ever.
// The reader sweep (executed against the default branches) is grep-clean for
// `GetVariants` in BOTH spec and charly — the only `Variants`/`VmVariant` hits
// are the generated field/type declarations themselves, and `git log -S
// GetVariants` returns no commit on either repo's history. A future
// reintroduction must fail this test until it ships a real reader.
func TestDeployRejectsVariants(t *testing.T) {
	// An EMPTY variant body is deliberate: the pre-cutover schema accepts
	// `variants: {small: {}}` (the map and its #VmVariant are valid), so this
	// asserts the SURFACE is gone, not that some inner field is invalid. A body
	// carrying a field like `cpu:` would be rejected by the OLD #VmVariant too
	// (its shape fields were `cpus:`/`memory:`), so the test would pass before the
	// cutover for the wrong reason and gate nothing.
	if err := deployShape(t, `{from: "some-vm", variants: {small: {}}}`); err == nil {
		t.Fatal("the deleted `variants:` surface was accepted on a #Deploy body")
	}
}

// TestDeployRejectsDiskSize pins the deletion of the unreadable per-deploy
// `disk_size:`. A kind:vm template's disk_size builds the shared base disk ONCE
// and every deploy boots a read-only COW overlay of it, so a per-deploy value
// could never resize it. The field had zero readers and zero authors (tree-wide
// classification in the PR body) and was removed rather than parked. A future
// per-deploy disk mechanism must ship a real reader, which will require changing
// this test deliberately.
func TestDeployRejectsDiskSize(t *testing.T) {
	if err := deployShape(t, `{from: "some-vm", disk_size: "40G"}`); err == nil {
		t.Fatal("the deleted per-deploy `disk_size:` was accepted on a #Deploy body")
	}
}
