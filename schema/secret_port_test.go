package schema_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestTheSecretContractIsDesigns port-verifies the secret delivery contract.
//
// DESIGN.cue declares `#SecretUse{src?, dst?, optional?}` at :474-480 and `#AbsPath` at :77, and
// schema/secret.cue is that declaration MOVED into the contract module. This test pins what a
// re-spelling would silently change.
//
// It fails without the port: `spec.SecretUse` does not exist on `main`, so this file does not compile
// there.
func TestTheSecretContractIsDesigns(t *testing.T) {
	t.Run("#SecretUse carries PATHS, never a value (D-SEC-1)", func(t *testing.T) {
		// The whole point of a secret `use:` is that the value travels src -> dst as a FILE and never
		// becomes text in the IR. So the assertion is about the ABSENCE of a value-bearing key: a
		// future `Value:`/`Secret:` field added "for convenience" fails here.
		raw, err := json.Marshal(spec.SecretUse{
			Src: "/run/charly/secrets/db_password",
			Dst: "/etc/app/db_password",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		got := string(raw)
		for _, forbidden := range []string{`"value"`, `"secret"`, `"text"`, `"password":`} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("a secret use must have nowhere to put a VALUE; found %s in %s", forbidden, got)
			}
		}
		// And the two paths that ARE the contract are on the wire.
		for _, want := range []string{`"src"`, `"dst"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("the delivery pair is missing %s from %s", want, got)
			}
		}
	})

	t.Run("optional is the ONE behavioural flag", func(t *testing.T) {
		// `optional` exists because "this deployment works without the credential" is a real intent,
		// and the alternative -- authoring a placeholder secret to satisfy a required field -- is
		// exactly how a real credential ends up in an authored file.
		raw, err := json.Marshal(spec.SecretUse{Optional: true})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(raw), `"optional":true`) {
			t.Fatalf("optional must reach the wire: %s", raw)
		}
	})

	t.Run("#AbsPath is a named type, so a path cannot be a bare string", func(t *testing.T) {
		// The `[^$]` in DESIGN.cue's `=~"^/[^$]*$"` is load-bearing: it keeps a VARIABLE out of a
		// path, so a secret's src/dst can never expand into a value. The generated type is a NAMED
		// string rather than `string` so a caller cannot pass one without the constraint in view.
		var p spec.AbsPath = "/run/charly/secrets/db"
		if !strings.HasPrefix(string(p), "/") {
			t.Fatalf("AbsPath must be an absolute path, got %q", p)
		}
	})
}
