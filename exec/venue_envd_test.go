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
