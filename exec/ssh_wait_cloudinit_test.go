package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloudInitStatusScript_ReadsUnprivileged proves the fix for the 30-minute hang the
// check-cua-container-disk-vm bed exposed: the status script must read `cloud-init status`
// WITHOUT a password-prompting sudo, and must classify its terminal states (done / error /
// disabled) as terminal so the poll returns. The old script hardcoded `sudo cloud-init
// status`, which on a guest with cloud-init DISABLED (no NOPASSWD sudo) printed nothing and
// polled to the absolute cap.
//
// A fake `cloud-init` stub is placed on PATH (never the REAL service — this is the script's
// own pure logic, not a boundary) and `sudo` is stubbed to the measured password-required
// failure, so the script must classify UNPRIVILEGED.
func TestCloudInitStatusScript_ReadsUnprivileged(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}
	dir := t.TempDir()
	stub := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// `sudo` over a piped stdin (no -n) fails exactly as measured on the Cua guest.
	stub("sudo", "#!/bin/sh\necho 'sudo: a password is required' >&2\nexit 1\n")

	run := func() string {
		cmd := exec.Command("bash", "-c", cloudInitStatusScript)
		cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin", "HOME=" + dir}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("script failed: %v\n%s", err, out)
		}
		return string(out)
	}

	for _, tc := range []struct{ name, status, want string }{
		{"disabled (the Cua containerDisk)", "status: disabled", "status: disabled"},
		{"done", "status: done", "status: done"},
		{"error", "status: error", "status: error"},
		{"running", "status: running", "status: running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub("cloud-init", "#!/bin/sh\necho '"+tc.status+"'\n")
			if out := run(); !strings.Contains(out, tc.want) {
				t.Errorf("output %q does not contain %q", out, tc.want)
			}
		})
	}

	// No cloud-init binary at all -> the guest is treated as settled ("status: done").
	t.Run("absent cloud-init", func(t *testing.T) {
		if err := os.Remove(filepath.Join(dir, "cloud-init")); err != nil {
			t.Fatal(err)
		}
		if out := run(); !strings.Contains(out, "status: done") {
			t.Errorf("absent cloud-init must read as done, got %q", out)
		}
	})
}

// TestPollCloudInitSettled proves the OTHER half of the fix: that the TERMINAL classifier
// (which decides whether the poll RETURNS) treats the script's `status: disabled` — the Cua
// containerDisk's state — as settled, so the wait cannot run to its cap on that guest.
func TestPollCloudInitSettled(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		want bool
	}{
		{"disabled settles (the Cua containerDisk)", "status: disabled\n", true},
		{"done settles", "status: done\n", true},
		{"error settles", "status: error\n", true},
		{"running does NOT settle", "status: running\n", false},
		{"a blank/transient read does NOT settle", "", false},
		{"a not-startable read does NOT settle", "not started\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pollCloudInitSettled(tc.out); got != tc.want {
				t.Errorf("pollCloudInitSettled(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}
