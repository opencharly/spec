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
// matching #Vm (the template it overrides) and #VmVariant.
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
// CPU-quota string) and the libvirt #LibvirtCPU.cpus — different defs entirely,
// NOT #VmVariant, which this cutover aligned to `cpu:`. If a future edit
// reintroduces `cpus:` as an accepted #Deploy key, this fails.
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

// TestDeployVariantShapeAligned pins the third VM-shape surface: #VmVariant now
// reads `cpu:`/`ram:` (the pre-cutover spelling was `cpus:`/`memory:`), so the
// whole VM-shape family is consistent.
func TestDeployVariantShapeAligned(t *testing.T) {
	if err := deployShape(t, `{from: "some-vm", variants: {small: {cpu: 1, ram: "2G"}}}`); err != nil {
		t.Fatalf("#VmVariant with aligned `cpu:`/`ram:` was rejected: %v", err)
	}
	if err := deployShape(t, `{from: "some-vm", variants: {small: {cpus: 1, memory: "2G"}}}`); err == nil {
		t.Fatal("the retired #VmVariant `cpus:`/`memory:` spelling was accepted; the alignment is incomplete")
	}
}
