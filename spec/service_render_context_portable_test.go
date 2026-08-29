package spec

import "testing"

// BuildServiceRenderContext is the pure ServiceEntry -> render-context projection every
// init template renders against. A field the schema allows on an entry but this
// projection drops is unreachable from ANY template — authorable and inert — so the
// schema addition and this carry have to be asserted together.
func TestBuildServiceRenderContextCarriesPortableFields(t *testing.T) {
	entry := &ServiceEntry{
		Name:        "svc",
		Exec:        "/usr/bin/svc",
		Type:        "notify",
		Requires:    []string{"a.service", "b.socket"},
		RestartSec:  "5s",
		WatchdogSec: "30s",
		UnitOptions: map[string]map[string]any{
			"systemd": {
				"KillMode":         "process",
				"RuntimeDirectory": []string{"cstream", "cstream/leaders"},
			},
		},
	}
	ctx := BuildServiceRenderContext(entry, ServiceRenderContext{})

	if ctx.Type != "notify" {
		t.Errorf("Type = %q, want notify — systemd Type= is unreachable", ctx.Type)
	}
	if len(ctx.Requires) != 2 {
		t.Errorf("Requires = %v, want both entries — a hard dependency is unreachable", ctx.Requires)
	}
	if ctx.RestartSec != "5s" {
		t.Errorf("RestartSec = %q, want 5s", ctx.RestartSec)
	}
	if ctx.WatchdogSec != "30s" {
		t.Errorf("WatchdogSec = %q, want 30s", ctx.WatchdogSec)
	}
	sysd, ok := ctx.UnitOptions["systemd"]
	if !ok {
		t.Fatalf("UnitOptions lost the systemd key: %v — every init-specific directive is "+
			"unreachable", ctx.UnitOptions)
	}
	if sysd["KillMode"] != "process" {
		t.Errorf("UnitOptions[systemd][KillMode] = %v, want process", sysd["KillMode"])
	}
	// The list form is what systemd needs for repeatable directives; losing its shape
	// would collapse two RuntimeDirectory= lines into one unusable value.
	list, ok := sysd["RuntimeDirectory"].([]string)
	if !ok || len(list) != 2 {
		t.Errorf("UnitOptions[systemd][RuntimeDirectory] = %#v, want a 2-element list",
			sysd["RuntimeDirectory"])
	}
}

// An entry that sets none of them must leave the context's fields zero, so an existing
// candy renders byte-identically.
func TestBuildServiceRenderContextLeavesPortableFieldsUnsetWhenAbsent(t *testing.T) {
	ctx := BuildServiceRenderContext(&ServiceEntry{Name: "svc", Exec: "/usr/bin/svc"},
		ServiceRenderContext{})
	if ctx.Type != "" || ctx.RestartSec != "" || ctx.WatchdogSec != "" ||
		len(ctx.Requires) != 0 || len(ctx.UnitOptions) != 0 {
		t.Errorf("an entry setting none of the portable fields produced a non-zero context: "+
			"%+v — existing candies would stop rendering byte-identically", ctx)
	}
}

// wait_for has to reach the render context like the rest: a readiness precondition
// the templates cannot see is a precondition that never runs.
func TestBuildServiceRenderContextCarriesWaitFor(t *testing.T) {
	entry := &ServiceEntry{
		Name: "svc", Exec: "/usr/bin/svc",
		WaitFor: &ServiceWaitFor{
			Paths:   []string{"/tmp/xdg/wayland-1", "/run/broker.sock"},
			Timeout: "30s",
		},
	}
	ctx := BuildServiceRenderContext(entry, ServiceRenderContext{})
	if ctx.WaitFor == nil {
		t.Fatal("WaitFor is nil: the readiness precondition never reaches a template, " +
			"so the service starts before what it needs exists")
	}
	if len(ctx.WaitFor.Paths) != 2 || ctx.WaitFor.Timeout != "30s" {
		t.Errorf("WaitFor = %+v, want both paths and the timeout", ctx.WaitFor)
	}
	// A service without wait_for must stay nil so a template can omit the whole
	// pre-start branch rather than emitting an empty one.
	plain := BuildServiceRenderContext(&ServiceEntry{Name: "p", Exec: "/bin/p"},
		ServiceRenderContext{})
	if plain.WaitFor != nil {
		t.Errorf("WaitFor = %+v for an entry that declares none, want nil", plain.WaitFor)
	}
}
