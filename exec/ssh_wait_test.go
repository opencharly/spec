package exec

import "testing"

// TestIsPermanentSSHFailure: the WaitForSSH probe must abort IMMEDIATELY on a
// permanent failure (the managed alias missing, a stale known_hosts entry) via
// ErrPollFatal — polling can never recover, and burning the whole readiness cap
// on an unresolvable alias is the check-live hang regression. "Permission
// denied" is deliberately NOT permanent: a first-boot guest's authorized_keys
// is seeded by cloud-init AFTER sshd comes up, so auth refusal is transient.
func TestIsPermanentSSHFailure(t *testing.T) {
	permanent := []string{
		"ssh: Could not resolve hostname charly-check-charly-omarchy-dev-vm: Name or service not known",
		"ssh: Could not resolve hostname foo: Name or service not known",
		"Host key verification failed.",
		"REMOTE HOST IDENTIFICATION HAS CHANGED!",
	}
	for _, msg := range permanent {
		if !isPermanentSSHFailure(msg) {
			t.Fatalf("isPermanentSSHFailure(%q) = false, want true", msg)
		}
	}
	transient := []string{
		"Permission denied (publickey,password).",
		"Connection refused",
		"Connection timed out",
		"",
	}
	for _, msg := range transient {
		if isPermanentSSHFailure(msg) {
			t.Fatalf("isPermanentSSHFailure(%q) = true, want false", msg)
		}
	}
}
