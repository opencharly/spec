package schema_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// kindclusterSchema compiles the shipped schema and returns the compiled value.
func kindclusterSchema(t *testing.T) cue.Value {
	t.Helper()
	schemaSrc, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concatenating the shipped schema: %v", err)
	}
	v := cuecontext.New().CompileString(schemaSrc)
	if v.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", v.Err())
	}
	return v
}

// kindclusterValueUnifies unifies #KindclusterValue with the given CUE literal,
// returning the unification error (nil = accepted). #KindclusterValue is the
// host-side value gate for a `kindcluster:` node (template OR deploy shape).
func kindclusterValueUnifies(t *testing.T, literal string) error {
	t.Helper()
	v := kindclusterSchema(t)
	def := v.LookupPath(cue.ParsePath("#KindclusterValue"))
	if !def.Exists() {
		t.Fatal("#KindclusterValue is not defined in the shipped schema")
	}
	return def.Unify(v.Context().CompileString(literal)).Validate(cue.Concrete(false))
}

// kindclusterUnifies unifies #Kindcluster with the given literal.
func kindclusterUnifies(t *testing.T, literal string) error {
	t.Helper()
	v := kindclusterSchema(t)
	def := v.LookupPath(cue.ParsePath("#Kindcluster"))
	if !def.Exists() {
		t.Fatal("#Kindcluster is not defined in the shipped schema")
	}
	return def.Unify(v.Context().CompileString(literal)).Validate(cue.Concrete(false))
}

// A cluster-policy-only template (box empty) with every kind-specific knob set
// is accepted — the shape a `kindcluster:` TEMPLATE node authors.
func TestKindclusterAcceptsPolicyTemplate(t *testing.T) {
	literal := `{
		box: ""
		engine: "podman"
		node_image: "kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5"
		nodes: [
			{role: "control-plane", extra_port_mappings: [{container_port: 30080, host_port: 30080}]},
			{role: "worker"},
		]
		kubeconfig_context: "kind-kind"
		default_namespace: "apps"
		admission_policy: "restricted"
	}`
	if err := kindclusterUnifies(t, literal); err != nil {
		t.Fatalf("valid kindcluster template rejected: %v", err)
	}
}

// #KindclusterValue accepts the DEPLOY shape too (a `from:` cross-ref), routed by
// shape in the loader.
func TestKindclusterValueAcceptsDeployShape(t *testing.T) {
	if err := kindclusterValueUnifies(t, `{from: "kindcluster-base", engine: "docker"}`); err != nil {
		t.Fatalf("kindcluster deploy shape rejected: %v", err)
	}
}

// An unknown key is rejected (the def is CLOSED).
func TestKindclusterRejectsUnknownKey(t *testing.T) {
	if err := kindclusterUnifies(t, `{box: "", bogus_key: true}`); err == nil {
		t.Fatal("kindcluster with an unknown key must be rejected")
	}
}

// An unknown engine word is rejected (#EngineName is the closed vocabulary).
func TestKindclusterRejectsUnknownEngine(t *testing.T) {
	if err := kindclusterUnifies(t, `{box: "", engine: "containerd"}`); err == nil {
		t.Fatal("kindcluster with an unknown engine must be rejected")
	}
}

// The cluster-policy sub-blocks reuse the SAME defs the kubernetes template uses,
// so a kubernetes-style policy block is accepted verbatim.
func TestKindclusterAcceptsKubernetesPolicyBlocks(t *testing.T) {
	literal := `{
		box: ""
		storage: {class_default: "standard", access_mode_default: "ReadWriteOnce"}
		ingress: {enabled: true, class: "nginx"}
		image_default: {pull_policy: "IfNotPresent"}
		pod_default: {node_selector: {disk: "ssd"}}
		defaults: {labels: {"managed-by": "opencharly"}}
	}`
	if err := kindclusterUnifies(t, literal); err != nil {
		t.Fatalf("kindcluster with kubernetes policy blocks rejected: %v", err)
	}
}
