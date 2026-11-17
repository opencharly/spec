package schema_test

// capability_port_test.go — the R7 half of the capability-ask port.
//
// DESIGN.cue declares the §8/§9 capability surface in full (the M4 host words at :217-244, the
// table and its shapes at :466-537, the M3 driver table its `driver_api` row keys off at
// :113-146/:245-274, the role vocabulary its last row loops over at :1138). schema/capability.cue is
// that declaration MOVED into the contract module.
//
// WHAT THIS FILE PINS, and why each assertion can fail:
//
//   - the WORD SETS, exactly. This is not pedantry: three of DESIGN.cue's generic words are already
//     taken by unrelated concepts in other schema files, and CUE UNIFIES two definitions of one name
//     SILENTLY. Measured before this file existed: `_device` here plus schema/vm.cue's `_device`
//     evaluates to ["kvm","render","virtio-vga","virtio-vga-gl"] with exit 0 — neither `cue-gen` nor
//     `go build` refuses that, so a set-equality assertion is the ONLY thing that catches it.
//   - the TABLE-DERIVED shapes (#Need/#Use/#Provide type keys), so a hand-written list cannot stand
//     in for the table.
//   - the LEASE placement (D-HW-8: only a leasable type's entry carries `exclusive`).
//   - #Candy's parameter entry carries `one_of` (D-SCH-7), which is the whole reason #CandyNeed
//     exists beside #Need.
//   - the Go wiring (#Candy/#CandyModel carry the pair as optional pointers).
//
// #AbsPath's `[^$]` is NOT re-pinned here: schema/secret_port_test.go:41 already holds it for the
// module (the secret port added that check first), and a second copy would be the duplication the
// port exists to avoid.

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue"

	"github.com/opencharly/spec/spec"
)

// TestTheCapabilityVocabulariesAreDesigns pins every word table the capability surface names, through
// the CONTRACT's behaviour rather than through a lookup: every word DESIGN.cue gives the table must be
// ACCEPTED by #Need and a word it does not give must be REFUSED.
//
// Behaviour is the assertion that matters here, because three of DESIGN.cue's generic words are taken
// by unrelated concepts in other schema files and CUE merges two definitions of one name SILENTLY
// (measured: `_device` plus schema/vm.cue's `_device` evaluates to ["kvm","render","virtio-vga",
// "virtio-vga-gl"] with exit 0). A merged table makes the foreign word pass, so the REFUSED half is
// what catches it — and neither cue-gen nor go build refuses the merge.
func TestTheCapabilityVocabulariesAreDesigns(t *testing.T) {
	v := compileShipped(t)
	need := v.LookupPath(cue.ParsePath("#Need"))
	if !need.Exists() {
		t.Fatal("#Need does not resolve in the shipped schema")
	}

	for _, c := range []struct {
		typ     string
		accept  []string
		refuse  []string
		entry   string
		because string
	}{
		{"gpu", []string{"nvidia", "amd", "intel"}, []string{"foo"}, `{}`, "the three vendor words (D-HW-9)"},
		{"device", []string{"kvm", "render", "fuse", "tun", "vhost_net", "vsock", "hwrng", "kfd"},
			[]string{"virtio-vga", "9p"}, `{}`, "the host device nodes (§9)"},
		{"cpu", []string{"core"}, []string{"socket"}, `{count: 2}`, "the cpu words"},
		{"memory", []string{"ram", "shm"}, []string{"vram"}, `{size: "1G"}`, "the memory words"},
		{"storage", []string{"root"}, []string{"home"}, `{size: "10G"}`, "the storage words"},
		{"nesting", []string{"container", "vm", "userns"}, []string{"kvm"}, `{}`, "the nesting words"},
		{"isolation", []string{"microvm"}, []string{"vm"}, `{}`, "the isolation word"},
		{"privilege", []string{"rootful"}, []string{"rootless"}, `{}`, "the privilege word"},
		{"driver_api", []string{"podman", "docker", "containerd", "libvirt", "qemu", "krun", "firecracker", "kind", "k3s", "pi", "tmux", "secret_service", "gpg", "file"},
			[]string{"virtiofs", "9p"}, `{}`, "the driver table's own words (D-DRV-1)"},
	} {
		for _, word := range c.accept {
			if err := unifiesWith(need, `{`+c.typ+`: {"`+word+`": `+c.entry+`}}`); err != nil {
				t.Errorf("#Need refused %s.%s, which is one of %s: %v", c.typ, word, c.because, err)
			}
		}
		for _, word := range c.refuse {
			if err := unifiesWith(need, `{`+c.typ+`: {"`+word+`": `+c.entry+`}}`); err == nil {
				t.Errorf("#Need ACCEPTED %s.%s — that word is not %s, so this table was merged with "+
					"another file's definition of the same name (CUE unifies silently)", c.typ, word, c.because)
			}
		}
	}

	// The table has exactly the 16 DESIGN rows plus the five role rows: a type nobody declared is refused.
	if err := unifiesWith(need, `{nosuchtype: {x: {}}}`); err == nil {
		t.Error("#Need accepted a capability type that is not a row of _capability")
	}
	// A role row's KEY is a #Word — any schema word, which is what `<role>:<word>` means (D-ROLE-6) — so a
	// non-#Word must still be refused.
	if err := unifiesWith(need, `{kind: {"Not-A-Word": {}}}`); err == nil {
		t.Error("#Need accepted a role key that is not a #Word")
	}

	// The THIRD collision: `_accessWord` is DESIGN.cue's seven venue accesses (D-VENUE), but
	// schema/deploy.cue declares an unrelated volume-access vocabulary under that name, so CUE merges
	// them and #DriverRow admits a volume mode as a venue access.
	driver := v.LookupPath(cue.ParsePath("#DriverRow"))
	if !driver.Exists() {
		t.Fatal("#DriverRow does not resolve in the shipped schema")
	}
	if err := unifiesWith(driver, `{subsystem: "pod", access: ["single-writer"]}`); err == nil {
		t.Error("#DriverRow accepted `single-writer` as a venue access — that is a VOLUME access mode, " +
			"so _access/_accessWord in schema/deploy.cue was merged with DESIGN.cue's venue accesses")
	}
	for _, a := range []string{"exec", "copy", "log", "console", "display", "forward", "power"} {
		if err := unifiesWith(driver, `{subsystem: "pod", access: ["`+a+`"]}`); err != nil {
			t.Errorf("#DriverRow refused %q, one of the seven venue accesses (D-VENUE): %v", a, err)
		}
	}
}

// unifiesWith reports whether def accepts the CUE literal expr.
//
// Concrete is required: a missing REQUIRED field leaves a value INCOMPLETE rather than bottom, so a
// plain Validate() accepts it (measured: a candy parameter without `one_of` validates clean and only
// fails under cue.Concrete).
func unifiesWith(def cue.Value, expr string) error {
	probe := def.Context().CompileString(expr)
	if probe.Err() != nil {
		return probe.Err()
	}
	return def.Unify(probe).Validate(cue.Concrete(true))
}

// TestTheNeedUseProvideAreDerivedFromTheTable proves the three shapes are the TABLE's rows and not a
// second hand-written list: every one of the 21 rows appears as a type key in #Need and #Use, and the
// nine rows that declare `provide:` appear in #Provide while the rest do not.
//
// The keys are read from the GENERATED types rather than from the CUE value: #Need's fields are
// comprehension-generated and OPTIONAL, so CUE's Fields()/LookupPath see none of them (measured), while
// the generated Go struct carries exactly the type keys.
func TestTheNeedUseProvideAreDerivedFromTheTable(t *testing.T) {
	all := []string{"env", "parameter", "secret", "endpoint", "network", "harness", "gpu", "device",
		"cpu", "memory", "storage", "mount", "nesting", "isolation", "privilege", "driver_api",
		"kind", "verb", "subcommand", "type", "api"}
	provided := []string{"env", "parameter", "network", "endpoint", "kind", "verb", "subcommand", "type", "api"}

	for _, c := range []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"spec.Need", reflect.TypeOf(spec.Need{}), all},
		{"spec.Use", reflect.TypeOf(spec.Use{}), all},
		{"spec.Provide", reflect.TypeOf(spec.Provide{}), provided},
	} {
		got := jsonKeys(c.typ)
		if !reflect.DeepEqual(got, sortedCopy(c.want)) {
			t.Fatalf("%s carries %v, want exactly %v\n"+
				"the keys are the rows of _capability, so a row added or dropped must appear or "+
				"disappear here (a hand-written list would not move)", c.name, got, sortedCopy(c.want))
		}
	}
	// A row that declares no `provide:` must not be present.
	if _, ok := jsonField(reflect.TypeOf(spec.Provide{}), "secret"); ok {
		t.Error("spec.Provide carries `secret` — its _capability row declares no `provide:`")
	}
}

// TestTheLeaseFieldIsOnlyOnLeasableTypes pins D-HW-8's placement: `exclusive` exists on a need entry
// of a leasable type and NOT on any other.
func TestTheLeaseFieldIsOnlyOnLeasableTypes(t *testing.T) {
	v := compileShipped(t)
	need := v.LookupPath(cue.ParsePath("#Need"))

	cpu := need.Unify(need.Context().CompileString(`{cpu: {core: {count: 2}}}`))
	if err := cpu.Validate(); err != nil {
		t.Fatalf("a cpu need with a count must validate: %v", err)
	}
	if !cpu.LookupPath(cue.ParsePath("cpu.core.exclusive")).Exists() {
		t.Error("cpu.core carries no `exclusive` — a leasable type's entry must (D-HW-8)")
	}

	env := need.Unify(need.Context().CompileString(`{env: {FOO: {}}}`))
	if err := env.Validate(); err != nil {
		t.Fatalf("an env need must validate: %v", err)
	}
	if env.LookupPath(cue.ParsePath("env.FOO.exclusive")).Exists() {
		t.Error("env.FOO carries `exclusive` — only a leasable type's entry may (D-HW-8)")
	}
}

// TestACandyParameterNeedMustCarryOneOf pins D-SCH-7 — the one reason #CandyNeed exists beside #Need.
func TestACandyParameterNeedMustCarryOneOf(t *testing.T) {
	v := compileShipped(t)
	candyNeed := v.LookupPath(cue.ParsePath("#CandyNeed"))
	if err := candyNeed.Validate(); err != nil {
		t.Fatalf("#CandyNeed does not validate on its own: %v", err)
	}
	if err := unifiesWith(candyNeed, `{parameter: {p: {description: "no bounded choice"}}}`); err == nil {
		t.Error("a candy parameter need WITHOUT `one_of` was accepted — D-SCH-7 makes it a bounded choice")
	}
	if err := unifiesWith(candyNeed, `{parameter: {p: {one_of: ["a", "b"]}}}`); err != nil {
		t.Errorf("a candy parameter need WITH `one_of` was refused: %v", err)
	}
}

// TestTheEndpointShapeIsDesigns pins #Endpoint's default transport and bounded port.
func TestTheEndpointShapeIsDesigns(t *testing.T) {
	v := compileShipped(t)
	ep := v.LookupPath(cue.ParsePath("#Endpoint"))
	ok := ep.Unify(ep.Context().CompileString(`{port: 8080}`))
	if err := ok.Validate(); err != nil {
		t.Fatalf("#Endpoint{port: 8080} must validate: %v", err)
	}
	var transport string
	if err := ok.LookupPath(cue.ParsePath("transport")).Decode(&transport); err != nil || transport != "tcp" {
		t.Errorf("transport = %q (err %v), want the DESIGN default \"tcp\"", transport, err)
	}
	for _, bad := range []string{`{port: 0}`, `{port: 65536}`, `{port: 8080, transport: "sctp"}`} {
		if err := unifiesWith(ep, bad); err == nil {
			t.Errorf("#Endpoint accepted %s — the port and transport vocabularies are closed", bad)
		}
	}
}

// TestCandyAndCandyModelCarryTheAskPair pins the wiring itself: the two Go structs carry `need`/`use`
// as OPTIONAL pointers (the `@go(...,optional=nillable)` spelling beside .capability), and their JSON
// keys are the authored words.
func TestCandyAndCandyModelCarryTheAskPair(t *testing.T) {
	for _, c := range []struct {
		name string
		typ  reflect.Type
	}{{"#Candy", reflect.TypeOf(spec.Candy{})}, {"#CandyModel", reflect.TypeOf(spec.CandyModel{})}} {
		for _, want := range []struct{ key, goType string }{
			{"need", "*spec.CandyNeed"},
			{"use", "*spec.CandyUse"},
		} {
			f, ok := jsonField(c.typ, want.key)
			if !ok {
				t.Fatalf("%s has no `%s` field — the authored pair must reach the generated type", c.name, want.key)
			}
			if got := f.Type.String(); got != want.goType {
				t.Errorf("%s.%s is %s, want %s (an optional struct: @go(...,optional=nillable))", c.name, f.Name, got, want.goType)
			}
		}
	}
}

// jsonKeys returns the struct's JSON keys, sorted.
func jsonKeys(t reflect.Type) []string {
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" {
			continue
		}
		out = append(out, strings.Split(tag, ",")[0])
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// jsonField finds the struct field whose JSON key is key.
func jsonField(t reflect.Type, key string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" {
			continue
		}
		if name := strings.Split(tag, ",")[0]; name == key {
			return f, true
		}
	}
	return reflect.StructField{}, false
}
