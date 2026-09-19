package exec

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestAppendHopForFlatPathCarriesEngine pins that a container-exec hop carries the
// node's ENGINE as DATA on the Jump (node.Engine is #EngineName): a nerdctl deploy
// must yield JumpContainerExec with Engine "nerdctl", not a podman hop. This is the
// branch that the engine-provider-class cutover rewrote (the former
// JumpPodmanExec/JumpDockerExec enum arms became one data-carrying jump); without
// the Engine field the value would silently default to podman in engineBinary().
func TestAppendHopForFlatPathCarriesEngine(t *testing.T) {
	node := &spec.Deploy{
		Target:  "pod",
		Engine:  spec.EngineName("nerdctl"),
		Descent: &spec.DescentDescriptor{Transport: "container-exec"},
	}

	chain, err := AppendHopForFlatPath(ShellExecutor{}, node, "web_db", "db")
	if err != nil {
		t.Fatalf("AppendHopForFlatPath: %v", err)
	}
	nested, ok := chain.(*NestedExecutor)
	if !ok {
		t.Fatalf("chain type = %T, want *NestedExecutor", chain)
	}
	if nested.Jump.Kind != JumpContainerExec {
		t.Fatalf("jump kind = %d, want %d", nested.Jump.Kind, JumpContainerExec)
	}
	if nested.Jump.Engine != "nerdctl" {
		t.Fatalf("jump engine = %q, want %q (the node's engine must ride the jump as data)", nested.Jump.Engine, "nerdctl")
	}
	if nested.Jump.Target != "charly-web_db" {
		t.Fatalf("jump target = %q, want %q", nested.Jump.Target, "charly-web_db")
	}
	if got := nested.Jump.engineBinary(); got != "nerdctl" {
		t.Fatalf("engineBinary() = %q, want %q", got, "nerdctl")
	}

	// The empty engine is the documented default: engineBinary() falls back to
	// podman, but the Jump itself still carries the empty (unset) engine value.
	unset := &spec.Deploy{
		Target:  "pod",
		Descent: &spec.DescentDescriptor{Transport: "container-exec"},
	}
	chain, err = AppendHopForFlatPath(ShellExecutor{}, unset, "plain", "plain")
	if err != nil {
		t.Fatalf("AppendHopForFlatPath(empty engine): %v", err)
	}
	if got := chain.(*NestedExecutor).Jump.Engine; got != "" {
		t.Fatalf("unset engine = %q, want empty", got)
	}
	if got := chain.(*NestedExecutor).Jump.engineBinary(); got != defaultContainerEngine {
		t.Fatalf("engineBinary() on unset = %q, want %q", got, defaultContainerEngine)
	}
}
