package container

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// openFDs counts this process's open descriptors. A leaked pipe PAIR moves this by 2.
func openFDs(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("no /proc/self/fd on this platform: %v", err)
	}
	return len(ents)
}

// TestStreamLoad_LoadStartFailureLeaksNoFDs — when the LOAD side fails to start, the pipe
// pair save.StdoutPipe() created has no other owner. Nothing else will close it, and the
// venue transfer retries once on a torn overlay, so a leak here is per-attempt.
//
// Discriminating: with the `pipe.Close()` on that error path removed, the count rises by 2
// per call and this fails. The loop makes a single stray descriptor unmistakable rather
// than lost in noise.
func TestStreamLoad_LoadStartFailureLeaksNoFDs(t *testing.T) {
	before := openFDs(t)
	for range 8 {
		save := exec.Command("printf", "x")
		load := exec.Command("/nonexistent/definitely-not-a-binary")
		if err := StreamLoad(save, load); err == nil {
			t.Fatal("StreamLoad returned nil though the load side could not start")
		}
	}
	if after := openFDs(t); after > before+1 {
		t.Fatalf("open fds %d -> %d after 8 failed starts: descriptors leaked per attempt", before, after)
	}
}

// TestStreamLoad_SaveStartFailureDoesNotOrphanTheLoadChild — the load side is ALREADY
// RUNNING when save.Start() fails. Returning without reaping it leaves a live process; for
// `charly box load` that is an exec session inside a deployed container, which nothing ever
// comes back for.
//
// Discriminating: with the Kill/Wait removed, `sleep 30` survives the call and the signal-0
// probe below finds it alive, so this fails. It asserts the process is gone rather than
// asserting StreamLoad merely returned an error — an error is returned either way.
func TestStreamLoad_SaveStartFailureDoesNotOrphanTheLoadChild(t *testing.T) {
	save := exec.Command("/nonexistent/definitely-not-a-binary")
	load := exec.Command("sleep", "30")

	err := StreamLoad(save, load)
	if err == nil {
		t.Fatal("StreamLoad returned nil though the save side could not start")
	}
	if load.Process == nil {
		t.Fatal("the load side never started; this test cannot discriminate")
	}
	pid := load.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	// Signal 0 probes liveness without delivering anything. Reaped -> ESRCH.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return // gone: killed and waited
		}
		if time.Now().After(deadline) {
			t.Fatalf("load child pid %d still alive after StreamLoad returned: orphaned", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
