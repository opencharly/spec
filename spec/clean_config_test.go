package spec

// clean_config_test.go — the per-host `clean:` section: the machine-scoped store
// ceiling D-STORE-7's sweep honours.
//
// The requirement this test exists for is CLOSEDNESS. The per-host file's sections
// (`cache:`, `ledger:`, `system:`, and now `clean:`) are declared fields of the
// #NodeDoc gate, so before this section existed an authored block failed the load as
// an unknown key — and a misspelt key INSIDE it must keep failing rather than being
// silently ignored, because the retention plugin would then fall back to its 32 GiB
// default and the operator's 256 GiB would never apply. That is why `clean:` is TYPED
// in the gate where its older siblings are `_`.
//
// The gate below compiles the SAME concatenation the runtime gate runs (schemaconcat
// over the embedded schema — one concatenation contract), and the decode arms prove
// the value reaches BOTH per-host consumers: the unified document model and the
// deploy-config overlay.

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"gopkg.in/yaml.v3"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// cleanDoc compiles the shipped schema and returns the #NodeDoc gate, so every arm
// below unifies against the same gate the loader runs.
func cleanDoc(t *testing.T) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(schema.FS, ".", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	ctx := cuecontext.New()
	compiled := ctx.CompileString(src)
	if compiled.Err() != nil {
		t.Fatalf("the shipped schema does not compile: %v", compiled.Err())
	}
	gate := compiled.LookupPath(cue.ParsePath("#NodeDoc"))
	if !gate.Exists() {
		t.Fatal("#NodeDoc is not defined in the shipped schema")
	}
	return gate
}

// TestCleanSectionPassesTheNodeDocGate — an authored `clean:` block with a 256 GiB
// ceiling is recognized by the closed document gate. Before this section existed the
// same document was an unknown key.
func TestCleanSectionPassesTheNodeDocGate(t *testing.T) {
	gate := cleanDoc(t)
	doc := gate.Context().CompileString(`{
		clean: {
			store_budget: {
				bytes: 274877906944
			}
		}
	}`)
	unified := gate.Unify(doc)
	if unified.Err() != nil {
		t.Fatalf("#NodeDoc gate rejected the clean section:\n%v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(false)); err != nil {
		t.Fatalf("#NodeDoc gate validation failed for the clean section:\n%v", err)
	}
}

// TestCleanSectionIsClosedAndBounded — the two ways an author gets it wrong must be
// LOAD ERRORS, not silent fallbacks: an undeclared key (typo) inside the section, and
// a ceiling that is not a positive byte count.
func TestCleanSectionIsClosedAndBounded(t *testing.T) {
	gate := cleanDoc(t)
	for _, arm := range []struct {
		name string
		doc  string
	}{
		{"undeclared key in the section", `{clean: {store_budgte: {bytes: 5}}}`},
		{"zero budget", `{clean: {store_budget: {bytes: 0}}}`},
		{"negative budget", `{clean: {store_budget: {bytes: -1}}}`},
	} {
		t.Run(arm.name, func(t *testing.T) {
			doc := gate.Context().CompileString(arm.doc)
			if err := doc.Err(); err != nil {
				t.Fatalf("the arm's own document does not compile: %v", err)
			}
			if err := gate.Unify(doc).Validate(cue.Concrete(false)); err == nil {
				t.Fatalf("the #NodeDoc gate accepted %s — the section must be closed and bounded", arm.name)
			}
		})
	}
}

// TestCleanSectionReachesBothPerHostModels — the value survives the YAML decode into
// the unified document model AND the deploy-config overlay (the two per-host structs
// that carry every section), and an absent section stays ABSENT: nil, never a default,
// because the one fallback lives in the retention plugin.
func TestCleanSectionReachesBothPerHostModels(t *testing.T) {
	const authored = `
clean:
  store_budget:
    bytes: 274877906944
`
	var dc DeployConfig
	if err := yaml.Unmarshal([]byte(authored), &dc); err != nil {
		t.Fatalf("decode the per-host overlay: %v", err)
	}
	if dc.Clean == nil || dc.Clean.StoreBudget == nil {
		t.Fatalf("the deploy-config overlay dropped the clean section: %+v", dc.Clean)
	}
	if got := dc.Clean.StoreBudget.Bytes; got != 274877906944 {
		t.Errorf("DeployConfig clean.store_budget.bytes = %d, want 274877906944", got)
	}

	var uf UnifiedFile
	if err := yaml.Unmarshal([]byte(authored), &uf); err != nil {
		t.Fatalf("decode the unified document: %v", err)
	}
	if uf.Clean == nil || uf.Clean.StoreBudget == nil {
		t.Fatalf("the unified document model dropped the clean section: %+v", uf.Clean)
	}
	if got := uf.Clean.StoreBudget.Bytes; got != 274877906944 {
		t.Errorf("UnifiedFile clean.store_budget.bytes = %d, want 274877906944", got)
	}

	// ABSENT MUST STAY ABSENT: no CUE default, no Go default. A decoded document with no
	// `clean:` block resolves to nil, which is what lets the plugin own the 32 GiB
	// fallback instead of inheriting a second copy of the number.
	var absent DeployConfig
	if err := yaml.Unmarshal([]byte("deploy: {}\n"), &absent); err != nil {
		t.Fatalf("decode a document without a clean section: %v", err)
	}
	if absent.Clean != nil {
		t.Errorf("an absent clean section decoded to %+v, want nil", absent.Clean)
	}
}
