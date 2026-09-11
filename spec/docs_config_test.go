package spec

// docs_config_test.go — the `docs` kind ROUTING + round-trip coverage for the
// #DocsConfig contract (schema/docs.cue). Classification mirrors the
// marketplace:/skill:/hook: kinds EXACTLY (R3 — no special-casing for docs
// anywhere):
//
//   - a document carrying a `docs:` node is a plain node-form document
//     (ClassifyDoc → DocShapeNode), never a directive-only or empty shape;
//   - the unified #NodeDoc gate accepts the node structurally — `docs` is a
//     non-directive top-level key on the generic open #Node spine, so the walk
//     surfaces `<name>: {docs: <body>}` as one entity (sdk/candywalk decodes it
//     into spec.DocsConfig exactly like readKinds decodes skill:/hook:/marketplace:
//     entities into spec.Skill/spec.Hook/spec.Marketplace);
//   - `docs` gets NO reserved-directive entry in spec.DocDirectives (asserted
//     below — the moment it lands there, the loader stops treating it as a node
//     and this test fails, forcing the special-casing decision to be made
//     explicitly);
//   - the kind WORD itself is recognized dynamically by its registered ClassKind
//     provider host-side (the charly wave), like every other plugin-provided kind
//     — this module owns only the contract, tested here end to end.
//
// The #NodeDoc gate below compiles the SAME concatenation the runtime gate runs
// (schemaconcat over the embedded schema — R3, one concatenation contract).

import (
	"os"
	"reflect"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"gopkg.in/yaml.v3"

	"github.com/opencharly/spec/schema"
	"github.com/opencharly/spec/schemaconcat"
)

// authoredDocsNode is the real authoring shape: ONE charly.yml `docs:` node —
// a top-level entity named "docs" whose single kind discriminator is the `docs`
// word, carrying the full #DocsConfig body. The body exercises every field of the
// brief's contract.
const authoredDocsNode = `docs:
    docs:
        sources:
            compiled:
                enabled: true
                compiled_plugins_path: charly/charly.yml
                go_mod_path: charly/charly/go.mod
            release_repos: [plugin-review, plugin-pipeline]
            extra_repos: [plugin-gh]
        marketplace:
            path: marketplace
        projections:
            recipes: true
            cli: true
            providers: true
            candy: true
            box: true
            plugin: true
            landing: true
        output:
            hand_authored: [start, concepts, guides]
        landing:
            readme: README.md
        gates:
            site_links: true
            sidebar_links: true
            prune: true
`

// TestDocsNode_IsANodeFormDocument — the classifier accepts the docs-carrying
// document as ordinary node-form (not empty, not an error), exactly like any
// skill:/hook:/marketplace:-carrying document.
func TestDocsNode_IsANodeFormDocument(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(authoredDocsNode), &doc); err != nil {
		t.Fatalf("parse authored docs node: %v", err)
	}
	shape, err := ClassifyDoc(&doc)
	if err != nil {
		t.Fatalf("ClassifyDoc rejected the docs-carrying document: %v", err)
	}
	if shape != DocShapeNode {
		t.Fatalf("ClassifyDoc = %v, want DocShapeNode (the docs node must route as node-form, not a directive shape)", shape)
	}
}

// TestDocsNode_PassesTheNodeDocGate — the unified loader gate (#NodeDoc, the same
// validate-before-execute gate GateDoc runs host-side) accepts the `docs:` node
// structurally: `docs` is on the generic #Node spine, mirroring the skill/hook/
// marketplace kinds.
func TestDocsNode_PassesTheNodeDocGate(t *testing.T) {
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
	// The whole document unified against #NodeDoc — the gate's own operation.
	doc := compiled.Context().CompileString(`{
		docs: {
			docs: {
				sources: {compiled: {enabled: true}}
				gates: {site_links: true}
			}
		}
	}`)
	unified := gate.Unify(doc)
	if unified.Err() != nil {
		t.Fatalf("#NodeDoc gate rejected the docs node:\n%v", unified.Err())
	}
	if err := unified.Validate(cue.Concrete(false)); err != nil {
		t.Fatalf("#NodeDoc gate validation failed for the docs node:\n%v", err)
	}
}

// TestDocsIsNotADocDirective — the no-special-casing mirror (asserted, not
// assumed): `docs` must NOT appear in the reserved document-directive
// vocabulary. The instant it does, the loader consumes it as a directive instead
// of a node and this routing contract silently breaks.
func TestDocsIsNotADocDirective(t *testing.T) {
	for _, d := range DocDirectives {
		if d == "docs" {
			t.Fatal("docs was added to DocDirectives — it must stay a kind NODE on the generic #Node spine (a non-directive top-level key), exactly like skill/hook/marketplace")
		}
	}
}

// TestDocsConfig_GeneratedTypeRoundTrips — the generated Go types are the decode
// target: the authored kind body (what sdk/candywalk hands the docs plugin, the
// readKinds pattern) decodes into spec.DocsConfig verbatim and re-marshals to a
// byte-faithful equivalent (marshal → decode → DeepEqual), the same round trip
// every plugin projection does with spec.Skill/spec.Hook/spec.Marketplace.
func TestDocsConfig_GeneratedTypeRoundTrips(t *testing.T) {
	entity, err := docsEntity(t, authoredDocsNode)
	if err != nil {
		t.Fatalf("extract docs entity: %v", err)
	}

	var cfg DocsConfig
	if err := entity.Decode(&cfg); err != nil {
		t.Fatalf("decode docs body into spec.DocsConfig: %v", err)
	}

	// Every field of the brief's contract, asserted after the decode.
	if !cfg.Sources.Compiled.Enabled {
		t.Error("sources.compiled.enabled not decoded")
	}
	if cfg.Sources.Compiled.CompiledPluginsPath != "charly/charly.yml" {
		t.Errorf("compiled_plugins_path = %q", cfg.Sources.Compiled.CompiledPluginsPath)
	}
	if cfg.Sources.Compiled.GoModPath != "charly/charly/go.mod" {
		t.Errorf("go_mod_path = %q", cfg.Sources.Compiled.GoModPath)
	}
	if !reflect.DeepEqual(cfg.Sources.ReleaseRepos, []string{"plugin-review", "plugin-pipeline"}) {
		t.Errorf("release_repos = %v", cfg.Sources.ReleaseRepos)
	}
	if !reflect.DeepEqual(cfg.Sources.ExtraRepos, []string{"plugin-gh"}) {
		t.Errorf("extra_repos = %v", cfg.Sources.ExtraRepos)
	}
	if cfg.Marketplace.Path != "marketplace" {
		t.Errorf("marketplace.path = %q", cfg.Marketplace.Path)
	}
	if !cfg.Projections.Recipes || !cfg.Projections.Cli || !cfg.Projections.Providers || !cfg.Projections.Candy || !cfg.Projections.Box || !cfg.Projections.Plugin || !cfg.Projections.Landing {
		t.Errorf("projections = %+v, want all true", cfg.Projections)
	}
	if !reflect.DeepEqual(cfg.Output.HandAuthored, []string{"start", "concepts", "guides"}) {
		t.Errorf("output.hand_authored = %v", cfg.Output.HandAuthored)
	}
	if cfg.Landing.Readme != "README.md" {
		t.Errorf("landing.readme = %q", cfg.Landing.Readme)
	}
	if !cfg.Gates.SiteLinks || !cfg.Gates.SidebarLinks || !cfg.Gates.Prune {
		t.Errorf("gates = %+v, want all true", cfg.Gates)
	}

	// Round trip: marshal → decode → DeepEqual (the same projection path the
	// plugins run; omitempty means zero-value fields drop out, so the full
	// authored config must round-trip on its own values).
	re, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal spec.DocsConfig: %v", err)
	}
	var again DocsConfig
	if err := yaml.Unmarshal(re, &again); err != nil {
		t.Fatalf("re-decode marshaled spec.DocsConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg, again) {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", again, cfg)
	}
}

// docsEntity extracts the docs kind entity from an authored node-form document —
// the sdk/candywalk ReadEntityFile contract (top-level node name → mapping of kind
// discriminators; the docs value body is the node's kind value). Returns the
// kind-VALUE yaml node (Entity.Value), so the caller decodes it into the
// generated spec type exactly like the plugins do.
func docsEntity(t *testing.T, docText string) (*yaml.Node, error) {
	t.Helper()
	var doc map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(docText), &doc); err != nil {
		return nil, err
	}
	node, ok := doc["docs"]
	if !ok {
		return nil, os.ErrNotExist
	}
	if node.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "docs" {
			return node.Content[i+1], nil
		}
	}
	return nil, nil //nolint:nilerr // no docs discriminator: not an error, caller decides
}
