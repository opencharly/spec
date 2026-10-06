package exec

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestWithMachineVenuePreambleIsNoopOnEmpty proves a blank script is returned
// unchanged, so a no-op step pays nothing.
func TestWithMachineVenuePreambleIsNoopOnEmpty(t *testing.T) {
	if got := WithMachineVenuePreamble(""); got != "" {
		t.Fatalf("blank script changed: %q", got)
	}
}

// TestWithMachineVenuePreamblePrefixes proves the preamble is a POSIX-sh loop that
// sources the venue user's own env.d via an unexpanded $HOME (the VENUE expands
// it), and is semicolon-terminated so a command can concatenate.
func TestWithMachineVenuePreamblePrefixes(t *testing.T) {
	got := WithMachineVenuePreamble("printenv FOO")
	if !strings.HasPrefix(got, MachineVenuePreamble) {
		t.Fatalf("preamble not prefixed: %q", got)
	}
	if !strings.HasSuffix(got, "printenv FOO") {
		t.Fatalf("command not appended after the preamble: %q", got)
	}
	if !strings.HasSuffix(MachineVenuePreamble, "; ") {
		t.Fatalf("preamble must be semicolon-terminated so it can concatenate: %q", MachineVenuePreamble)
	}
	if !strings.Contains(MachineVenuePreamble, `"$HOME"`) {
		t.Fatalf("preamble must leave $HOME for the venue to expand: %q", MachineVenuePreamble)
	}
}

// TestShellExecutorMachineVenueSourcesEnvd is the charly#814 proof, live: a
// ShellExecutor with MachineVenue=true runs a NON-interactive command that sees an
// env.d file a candy's `env:` would have written, while the SAME executor with
// MachineVenue=false (the container-jump transport) does not — so the opt-in is
// exactly the machine/local venue and nothing else.
func TestShellExecutorMachineVenueSourcesEnvd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencharly", "env.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(home, ".config", "opencharly", "env.d", "dsh-tui.env")
	if err := os.WriteFile(envFile, []byte("export DSH_TUI_NO_LAUNCHPAD=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// MachineVenue=true: the command sees the candy env.
	out, _, exit, err := (ShellExecutor{MachineVenue: true}).RunCapture(context.Background(), "printenv DSH_TUI_NO_LAUNCHPAD")
	if err != nil {
		t.Fatalf("RunCapture(machine): %v", err)
	}
	if exit != 0 || strings.TrimSpace(out) != "1" {
		t.Fatalf("machine venue did not source env.d: out=%q exit=%d", out, exit)
	}

	// MachineVenue=false (the container-jump transport): the command does NOT.
	out, _, _, _ = (ShellExecutor{}).RunCapture(context.Background(), "printenv DSH_TUI_NO_LAUNCHPAD")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("a non-machine ShellExecutor must NOT source the host env.d; got %q", out)
	}
}

// TestShellExecutorMachineVenueUserNotRoot proves the preamble is on the UNPRIVILEGED
// leg (which is where a candy's user-scoped env.d applies) and NOT on the sudo leg: a
// candy's `env:` is the deploying user's, while `sudo` resets HOME to root's, so
// sourcing it there would be wrong. The dry-run render is the cheap witness of what
// each leg would feed the shell.
func TestShellExecutorMachineVenueUserNotRoot(t *testing.T) {
	m := ShellExecutor{MachineVenue: true}
	var buf strings.Builder
	// Capture the dry-run render of both legs.
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	_ = m.RunUser(context.Background(), "printenv CHARLY_ENVD_PROBE", spec.EmitOpts{DryRun: true})
	_ = m.RunSystem(context.Background(), "printenv CHARLY_ENVD_PROBE", spec.EmitOpts{DryRun: true})
	_ = w.Close()
	os.Stderr = old
	_, _ = io.Copy(&buf, r)
	rendered := buf.String()

	// Both legs' rendered scripts appear; split on the dry-run labels.
	userPart, rootPart, ok := strings.Cut(rendered, "[dry-run] sudo")
	if !ok {
		t.Fatalf("could not find the root dry-run render:\n%s", rendered)
	}
	if !strings.Contains(userPart, MachineVenuePreamble) {
		t.Errorf("the UNPRIVILEGED leg must carry the env.d preamble:\n%s", userPart)
	}
	if strings.Contains(rootPart, MachineVenuePreamble) {
		t.Errorf("the sudo leg must NOT carry the env.d preamble (user-scoped env; sudo resets HOME):\n%s", rootPart)
	}
}

// TestVenueFromDescriptor_ShellIsMachineVenue proves the round-trip preserves the
// fix: the ONLY producer of a Kind "shell" descriptor is a machine ShellExecutor, so
// a re-materialized shell venue is a machine venue (charly#814).
func TestVenueFromDescriptor_ShellIsMachineVenue(t *testing.T) {
	got, err := VenueFromDescriptor(spec.VenueDescriptor{Kind: "shell"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	se, ok := got.(ShellExecutor)
	if !ok {
		t.Fatalf("want ShellExecutor, got %#v", got)
	}
	if !se.MachineVenue {
		t.Fatalf("a re-materialized shell venue must be a machine venue")
	}
}

// TestSSHExecutorRunCaptureStdinCarriesPreamble is the B12 proof for the leg the check
// engine's `command:` probe actually reaches on a VM venue (charly#814): the #814 symptom
// venue is a VM, so `SSHExecutor.RunCapture`'s stdin is the path a VM check probe runs.
// A fake `ssh` on PATH captures the stdin it is handed; the test asserts the env.d
// preamble is there (and FAILS if the prefixing is removed).
func TestSSHExecutorRunCaptureStdinCarriesPreamble(t *testing.T) {
	dir := t.TempDir()
	captured := filepath.Join(dir, "stdin.txt")
	fakeSSH := filepath.Join(dir, "ssh")
	script := "#!/bin/sh\ncat > " + captured + "\nexit 0\n"
	if err := os.WriteFile(fakeSSH, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	e := &SSHExecutor{Host: "charly-vm"}
	if _, _, _, err := e.RunCapture(context.Background(), "printenv FOO"); err != nil {
		t.Fatalf("RunCapture: %v", err)
	}
	got, err := os.ReadFile(captured)
	if err != nil {
		t.Fatalf("the fake ssh captured nothing: %v", err)
	}
	if !strings.Contains(string(got), MachineVenuePreamble) {
		t.Fatalf("SSHExecutor.RunCapture stdin must carry the env.d preamble (the VM check-probe leg):\n%s", got)
	}
	if !strings.Contains(string(got), "printenv FOO") {
		t.Fatalf("the command was lost from the stdin:\n%s", got)
	}
}

// TestSSHExecutorMachineVenueUserNotRoot proves the SSH venue (always a machine venue)
// prefixes the env.d preamble on the UNPRIVILEGED leg and NOT the sudo leg — the same
// contract as the local ShellExecutor (a candy's `env:` is the guest USER's; `sudo`
// resets HOME to root's). The dry-run render shows what each leg would feed the guest
// shell.
func TestSSHExecutorMachineVenueUserNotRoot(t *testing.T) {
	e := &SSHExecutor{Host: "charly-vm"}
	var buf strings.Builder
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	_ = e.run(context.Background(), "printenv CHARLY_ENVD_PROBE", false, spec.EmitOpts{DryRun: true})
	_ = e.run(context.Background(), "printenv CHARLY_ENVD_PROBE", true, spec.EmitOpts{DryRun: true})
	_ = w.Close()
	os.Stderr = old
	_, _ = io.Copy(&buf, r)
	rendered := buf.String()

	userPart, rootPart, ok := strings.Cut(rendered, "[dry-run] ssh vm sudo")
	if !ok {
		t.Fatalf("could not find the root dry-run render:\n%s", rendered)
	}
	if !strings.Contains(userPart, MachineVenuePreamble) {
		t.Errorf("the UNPRIVILEGED ssh leg must carry the env.d preamble:\n%s", userPart)
	}
	if strings.Contains(rootPart, MachineVenuePreamble) {
		t.Errorf("the sudo ssh leg must NOT carry the env.d preamble (the guest user's env, not root's):\n%s", rootPart)
	}
}

// TestNestedExecutorPrepareJumpEnvPreamble proves the nested-in-guest case: a JumpSSH
// payload is prefixed with the env.d preamble (so it lands INSIDE the guest, where the
// guest user's env.d lives), while a JumpContainerExec payload is NOT (a container's env
// comes from the image's ENV directives, and it has no candy env.d to source).
func TestNestedExecutorPrepareJumpEnvPreamble(t *testing.T) {
	sshJump := &NestedExecutor{Parent: ShellExecutor{}, Jump: NestedJump{Kind: JumpSSH, Target: "user@guest:2222"}}
	out, err := sshJump.prepareJump("printenv FOO", false)
	if err != nil {
		t.Fatalf("prepareJump(JumpSSH): %v", err)
	}
	if !strings.Contains(out, MachineVenuePreamble) {
		t.Fatalf("a JumpSSH payload must carry the env.d preamble:\n%s", out)
	}
	if !strings.Contains(out, "printenv FOO") {
		t.Fatalf("the payload was lost in the wrap:\n%s", out)
	}

	containerJump := &NestedExecutor{Parent: ShellExecutor{}, Jump: NestedJump{Kind: JumpContainerExec, Engine: "podman", Target: "charly-pod"}}
	out, err = containerJump.prepareJump("printenv FOO", false)
	if err != nil {
		t.Fatalf("prepareJump(JumpContainerExec): %v", err)
	}
	if strings.Contains(out, MachineVenuePreamble) {
		t.Fatalf("a JumpContainerExec payload must NOT carry the env.d preamble (the container has the image's ENV):\n%s", out)
	}
}

// claim VenueFromDescriptor("shell") relies on (charly#814, B13/RDD). A re-materialized
// "shell" venue carries MachineVenue=true, so the placement is safe only if a
// CONTAINER-BOUND command cannot round-trip into a "shell" descriptor. It cannot: a
// container/nested jump is always a *NestedExecutor*, and DescriptorFromExecutor reports
// every jump as "container" (container-exec) or "" (ssh/virsh) — its bare ShellExecutor
// PARENT (the local host transport) is never itself described. Only a ShellExecutor used
// as an actual venue emits "shell", and that is the machine venue.
func TestDescriptorFromExecutor_JumpTransportsNeverEmitShellKind(t *testing.T) {
	cases := []struct {
		name string
		exec spec.DeployExecutor
	}{
		{"container-exec jump", ContainerChainFromDescriptor("podman", "charly-x-1-1")},
		{"nested SSH jump", &NestedExecutor{Parent: ShellExecutor{}, Jump: NestedJump{Kind: JumpSSH, Target: "charly-vm"}}},
		{"nested SSH jump over a machine parent", &NestedExecutor{Parent: ShellExecutor{MachineVenue: true}, Jump: NestedJump{Kind: JumpSSH, Target: "charly-vm"}}},
		{"SSH venue", &SSHExecutor{Host: "charly-vm"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DescriptorFromExecutor(tc.exec).Kind; got == "shell" {
				t.Fatalf("a jump transport reported Kind=%q — it would re-materialize as a machine venue and source the HOST env.d into a container/guest command: %#v", got, tc.exec)
			}
		})
	}

	// The positive half: the machine venue itself IS the "shell" descriptor, and its
	// round-trip preserves MachineVenue.
	if got := DescriptorFromExecutor(ShellExecutor{MachineVenue: true}).Kind; got != "shell" {
		t.Fatalf("a machine ShellExecutor must emit Kind=\"shell\", got %q", got)
	}
}
