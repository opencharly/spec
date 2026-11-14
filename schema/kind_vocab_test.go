package schema_test

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestResourceKinds_HasKubevirt gates the schemagen-derived deployable-kind vocabulary:
// kubevirt must be a #ResourceKind (so the loader nests its members and the Go side derives the
// deploy-target vocabulary from it). The vocabulary is the CONTRACT's (a word list, not a kind
// body), so it stays here after opencharly/spec#304 unit 6 moved the body itself into the plugin
// that serves the kind — this is the assertion that outlived its old file.
func TestResourceKinds_HasKubevirt(t *testing.T) {
	for _, w := range spec.ResourceKinds {
		if w == "kubevirt" {
			return
		}
	}
	t.Fatalf("spec.ResourceKinds = %v, want it to contain kubevirt", spec.ResourceKinds)
}
