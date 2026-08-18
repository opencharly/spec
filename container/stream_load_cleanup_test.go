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
	// `cat` reads stdin, so it exits on EOF rather than after a fixed sleep — the previous
	// `sleep 30` fixture made this test take 30s to say anything and ignored its stdin
	// entirely.
	//
	// Honest limit, recorded because I first claimed the opposite: this test canNOT
	// discriminate the Kill/Wait ORDER. Reversing it still passes in ~4ms, because
	// exec.Cmd.Start's deferred closeDescriptors closes save's write end when Start fails,
	// so the load side sees EOF and exits either way. What this test DOES discriminate is
	// the reap existing at all — drop the Kill/Wait block and the child survives the call.
	// The elapsed bound guards a future regression where the write end stays open.
	save := exec.Command("/nonexistent/definitely-not-a-binary")
	load := exec.Command("cat")

	started := time.Now()
	err := StreamLoad(save, load)
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("StreamLoad took %s to return: the load side was Waited on before being "+
			"Killed, so it blocked reading a pipe nobody will write to", elapsed)
	}
	if err == nil {
		t.Fatal("StreamLoad returned nil though the save side could not start")
	}
	if load.Process == nil {
		t.Fatal("the load side never started; this test cannot discriminate")
	}
	pid := load.Process.Pid
	// NO t.Cleanup kill here. StreamLoad already Waited this child, so the pid is FREE —
	// signalling it later is an unscoped kill by a stale identifier, which the OS may have
	// reassigned to an unrelated process. That is the same class as `pkill -x charly`
	// matching by name: an identifier that no longer denotes what you think it does.

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
