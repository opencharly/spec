package spec

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// The GPU-in-VM configuration surface: #LibvirtVideo and #LibvirtGraphics used to
// model a small fraction of what libvirt accepts, which is why Spike 3 could only be
// run by hand-injecting <qemu:override> XML. These tests pin the decoded shape,
// because every gap here fails the same way — the YAML is accepted, the field is
// dropped, and the domain boots with a configuration the author did not ask for.

// A pattern-constrained CUE key ([=~"^ua-"]) renders as an EMPTY Go struct under
// `cue exp gengotypes`, not a map — so every override would decode into nothing at
// all, silently. The key is typed [string] for exactly that reason. This test fails
// to COMPILE if the key is ever re-tightened, and fails at runtime if the field stops
// carrying values.
func TestQemuOverrideDecodesAsAMap(t *testing.T) {
	var dom LibvirtDomain
	const src = `
qemu_override:
  ua-gpu:
    drm_native_context: true
    hostmem: 4G
    max_outputs: 1
`
	if err := yaml.Unmarshal([]byte(src), &dom); err != nil {
		t.Fatalf("decode: %v", err)
	}
	props, ok := dom.QemuOverride["ua-gpu"]
	if !ok {
		t.Fatalf("qemu_override lost the ua-gpu device; got %#v", dom.QemuOverride)
	}
	// The three CUE-declared scalar kinds must survive as their Go kinds — the
	// libvirt <qemu:property> element requires a type= attribute, so a bool that
	// decoded as the string "true" would be emitted with the wrong type.
	if v, ok := props["drm_native_context"].(bool); !ok || !v {
		t.Errorf("drm_native_context = %#v (%T), want bool true", props["drm_native_context"], props["drm_native_context"])
	}
	if v, ok := props["hostmem"].(string); !ok || v != "4G" {
		t.Errorf("hostmem = %#v (%T), want string \"4G\"", props["hostmem"], props["hostmem"])
	}
	if v, ok := props["max_outputs"].(int); !ok || v != 1 {
		t.Errorf("max_outputs = %#v (%T), want int 1", props["max_outputs"], props["max_outputs"])
	}
}

// Every attribute libvirtxml's DomainVideoModel / DomainVideoAccel / DomainVideoDriver
// carry must be reachable from YAML. Before this cutover the struct stopped at
// model/vram/heads/accel3d/primary, which is why `blob` and `device` — the two
// attributes a native-context guest cannot boot without — had no expression at all.
func TestLibvirtVideoCarriesTheFullModelSurface(t *testing.T) {
	var v LibvirtVideo
	const src = `
model: virtio
device: virtio-gpu-gl
ram: 65536
vram: 262144
vram64: 262144
vgamem: 16384
heads: 1
blob: true
edid: true
accel3d: true
accel2d: false
render_node: /dev/dri/renderD128
primary: true
alias: ua-gpu
resolution: {x: 1920, y: 1080}
driver: {name: vhostuser, vgaconf: io, iommu: true, ats: false, packed: true, page_per_vq: false}
`
	if err := yaml.Unmarshal([]byte(src), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.Device != "virtio-gpu-gl" {
		t.Errorf("device = %q — without it, model:virtio emits plain virtio-vga, which has no GL", v.Device)
	}
	if v.Blob == nil || !*v.Blob {
		t.Error("blob did not decode; a native-context or venus guest cannot work without it")
	}
	if v.Alias != "ua-gpu" {
		t.Errorf("alias = %q; qemu_override can target no device without it", v.Alias)
	}
	for _, f := range []struct {
		name string
		got  int
		want int
	}{
		{"ram", v.Ram, 65536}, {"vram", v.VRAM, 262144}, {"vram64", v.VRAM64, 262144},
		{"vgamem", v.VGAMem, 16384}, {"heads", v.Heads, 1},
	} {
		if f.got != f.want {
			t.Errorf("%s = %d, want %d", f.name, f.got, f.want)
		}
	}
	if v.Resolution == nil || v.Resolution.X != 1920 || v.Resolution.Y != 1080 {
		t.Errorf("resolution = %#v, want 1920x1080", v.Resolution)
	}
	if v.Driver == nil || v.Driver.Name != "vhostuser" || v.Driver.IOMMU == nil || !*v.Driver.IOMMU {
		t.Errorf("driver = %#v, want name=vhostuser iommu=true", v.Driver)
	}
	// accel2d:false and ats:false must decode as an explicit false, not as absent —
	// the renderer emits accel2d='no', which is not the same XML as omitting it.
	if v.Accel2D == nil || *v.Accel2D {
		t.Error("accel2d:false decoded as nil; a tri-state field must distinguish false from unset")
	}
	if v.Driver.ATS == nil || *v.Driver.ATS {
		t.Error("driver.ats:false decoded as nil")
	}
}

// gl was a bare string, so it could only ever reach spice's enable= attribute and had
// no way to express rendernode= — the attribute that points virtio-gpu at a specific
// host DRM node, and the only reason a GPU-in-VM candy touches <gl> at all.
func TestLibvirtGraphicsGLIsStructured(t *testing.T) {
	var g LibvirtGraphics
	const src = `
type: egl-headless
gl: {render_node: /dev/dri/renderD128}
`
	if err := yaml.Unmarshal([]byte(src), &g); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if g.GL == nil {
		t.Fatal("gl did not decode into a struct")
	}
	if g.GL.RenderNode != "/dev/dri/renderD128" {
		t.Errorf("gl.render_node = %q, want /dev/dri/renderD128", g.GL.RenderNode)
	}
}

// dbus is a real libvirt graphics type (libvirtxml models DomainGraphicDBus with its
// own address/p2p/gl) that the enum simply omitted.
func TestLibvirtGraphicsDBus(t *testing.T) {
	var g LibvirtGraphics
	const src = `
type: dbus
address: unix:path=/run/cstream/dbus
p2p: true
gl: {enable: true, render_node: /dev/dri/renderD128}
`
	if err := yaml.Unmarshal([]byte(src), &g); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if g.Address == "" || g.P2P == nil || !*g.P2P {
		t.Errorf("dbus address/p2p did not decode: address=%q p2p=%v", g.Address, g.P2P)
	}
	if g.GL == nil || g.GL.Enable == nil || !*g.GL.Enable {
		t.Errorf("dbus gl.enable did not decode: %#v", g.GL)
	}
}

// A CUE cross-rule written as `if <abstract> { field?: _|_ }` makes
// `cue exp gengotypes` emit `type X any` — it evaluates the definition with the
// discriminant still abstract, so the field kinds reduce to bottom and the generator
// degrades the whole struct. That still COMPILES, so nothing catches it: every value
// silently decodes into an untyped map instead of the struct.
//
// These conversions fail to build if any of these types is ever degraded to `any`,
// which is the only signal this failure mode gives.
func TestGpuSurfaceTypesAreStructsNotAny(t *testing.T) {
	_ = LibvirtGraphics{Type: "spice"}
	_ = LibvirtGraphicsGL{RenderNode: "/dev/dri/renderD128"}
	_ = LibvirtVideo{Model: "virtio"}
	_ = LibvirtVideoResolution{X: 1920, Y: 1080}
	_ = LibvirtVideoDriver{Name: "vhostuser"}
	_ = LibvirtDomain{QemuOverride: map[string]map[string]any{"ua-gpu": {"blob": true}}}
}

// The two closed vocabularies are transcribed from libvirt's own RNG
// (/usr/share/libvirt/schemas/domaincommon.rng), so this test is the place a drift between
// charly's schema and libvirt's would show up. Both were plain `string` when the fields
// landed, which meant an invalid value was accepted here and rejected at DEFINE time, with
// an RNG error that blames <devices> rather than the attribute.
func TestLibvirtVideoClosedVocabularies(t *testing.T) {
	// domaincommon.rng, the <model type='virtio'> group.
	for _, dev := range []string{
		"virtio-vga", "virtio-vga-gl", "virtio-gpu", "virtio-gpu-gl",
		"vhost-user-vga", "vhost-user-gpu",
	} {
		var v LibvirtVideo
		if err := yaml.Unmarshal([]byte("model: virtio\ndevice: "+dev+"\n"), &v); err != nil {
			t.Fatalf("decode %s: %v", dev, err)
		}
		if v.Device != dev {
			t.Errorf("device %q did not decode", dev)
		}
	}
	// domaincommon.rng, the video <driver> element. "qxl" is the plausible wrong guess —
	// it is a valid video MODEL but not a driver name.
	var d LibvirtVideoDriver
	if err := yaml.Unmarshal([]byte("name: vhostuser\nvgaconf: io\n"), &d); err != nil {
		t.Fatalf("decode driver: %v", err)
	}
	if d.Name != "vhostuser" || d.VGAConf != "io" {
		t.Errorf("driver = %#v", d)
	}
}
