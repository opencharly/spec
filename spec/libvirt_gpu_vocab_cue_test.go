package spec

import (
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/opencharly/spec/schemaconcat"
)

// loadVmSchema compiles the SAME concatenation the runtime and `task cue:gen` use
// (schemaconcat.ConcatSchema — R3: one concatenation contract), so this test exercises the
// schema as it actually ships rather than a hand-written excerpt of it.
func loadVmSchema(t *testing.T) cue.Value {
	t.Helper()
	src, _, err := schemaconcat.ConcatSchema(os.DirFS(".."), "schema", nil)
	if err != nil {
		t.Fatalf("concat schema: %v", err)
	}
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return v
}

// The closure is the point of the change, and it lives ONLY in CUE: `cue exp gengotypes`
// renders every one of these fields as a plain Go `string`, so a Go-level round-trip cannot
// see the difference between an open and a closed vocabulary. A test that only decodes VALID
// values would pass identically with the enums reverted — it proves nothing.
//
// This validates concrete values against the compiled #LibvirtVideo / #LibvirtVideoDriver
// definitions, so every reject case below FAILS if the field goes back to `string`.
func TestLibvirtVideoVocabulariesAreClosed(t *testing.T) {
	schema := loadVmSchema(t)

	for _, tc := range []struct {
		name   string
		def    string
		value  string
		reject bool
	}{
		// #LibvirtVideo.device — libvirt's RNG type='virtio' group.
		{"device virtio-gpu-gl accepted", "#LibvirtVideo", `{model: "virtio", device: "virtio-gpu-gl"}`, false},
		{"device vhost-user-gpu accepted", "#LibvirtVideo", `{model: "virtio", device: "vhost-user-gpu"}`, false},
		// "qxl" is a valid video MODEL, which is exactly why it is the plausible wrong
		// value here — and the schema used to take it.
		{"device qxl rejected", "#LibvirtVideo", `{model: "qxl", device: "qxl"}`, true},
		{"device virtio-gpu-rutabaga rejected", "#LibvirtVideo", `{model: "virtio", device: "virtio-gpu-rutabaga"}`, true},
		{"device empty string rejected", "#LibvirtVideo", `{model: "virtio", device: ""}`, true},

		// #LibvirtVideoDriver.name — libvirt's RNG allows only these two.
		{"driver name qemu accepted", "#LibvirtVideoDriver", `{name: "qemu"}`, false},
		{"driver name vhostuser accepted", "#LibvirtVideoDriver", `{name: "vhostuser"}`, false},
		{"driver name qxl rejected", "#LibvirtVideoDriver", `{name: "qxl"}`, true},

		// #LibvirtVideoDriver.vgaconf.
		{"vgaconf io accepted", "#LibvirtVideoDriver", `{vgaconf: "io"}`, false},
		{"vgaconf off accepted", "#LibvirtVideoDriver", `{vgaconf: "off"}`, false},
		{"vgaconf bogus rejected", "#LibvirtVideoDriver", `{vgaconf: "bogus"}`, true},
		// yes/no is the neighbouring attributes' spelling, so it is the likely slip.
		{"vgaconf yes rejected", "#LibvirtVideoDriver", `{vgaconf: "yes"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := schema.LookupPath(cue.ParsePath(tc.def))
			if err := def.Err(); err != nil {
				t.Fatalf("lookup %s: %v", tc.def, err)
			}
			// The value must be compiled in the SAME context as the schema; unifying
			// across contexts is not meaningful.
			val := schema.Context().CompileString(tc.value)
			if err := val.Err(); err != nil {
				t.Fatalf("compile value: %v", err)
			}
			// Unify the concrete value into the definition and demand a valid instance —
			// the same judgement the loader makes on an authored file.
			unified := def.Unify(val)
			got := unified.Validate(cue.Concrete(false), cue.Final())

			if tc.reject && got == nil {
				t.Errorf("%s ACCEPTED %s — the vocabulary is not closed", tc.def, tc.value)
			}
			if !tc.reject && got != nil {
				t.Errorf("%s rejected a valid value %s: %v", tc.def, tc.value, got)
			}
			if tc.reject && got != nil && !strings.Contains(got.Error(), "device") &&
				!strings.Contains(got.Error(), "name") && !strings.Contains(got.Error(), "vgaconf") {
				t.Logf("note: rejection message does not name the field: %v", got)
			}
		})
	}
}
