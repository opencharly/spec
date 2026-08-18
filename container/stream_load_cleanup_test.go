package container

import (
	"context"
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
// Discriminating: with the Kill/Wait removed, the fixture survives the call and the signal-0
// probe below finds it alive, so this fails. It asserts the process is gone rather than
// asserting StreamLoad merely returned an error — an error is returned either way.
func TestStreamLoad_SaveStartFailureDoesNotOrphanTheLoadChild(t *testing.T) {
	// `cat` reads stdin, so it exits on EOF rather than after a fixed sleep — the previous
	// `sleep 30` fixture made this test take 30s to say anything and ignored its stdin
	// entirely.
	//
	// This test discriminates BOTH properties, and getting there took two wrong fixtures:
	//   - the reap existing at all — drop the Kill/Wait block and the child survives;
	//   - the reap ORDER — reverse it and this times out instead of returning in ~4ms.
	//
	// The second only became observable once the fixture stopped self-terminating. With
	// `sleep 30` it passed after 30s; with `cat` it passed in 4ms, because EOF arrives
	// regardless of order. Both made the ordering invisible, and the `cat` result briefly
	// convinced me the ordering did not matter at all.
	// The fixture must NOT terminate on its own — that is the probe this test failed twice.
	// `sleep 30` self-terminates on its own deadline; `cat` self-terminates on EOF, and EOF
	// arrives regardless of reap order because exec.Cmd.Start's deferred closeDescriptors
	// closes save's write end when Start fails. Either fixture makes the ORDER unobservable.
	//
	// This one reads from /dev/null and loops forever: it ignores the pipe, so no EOF can
	// reach it, and it has no internal deadline. Now Kill-then-Wait returns promptly while
	// Wait-then-Kill blocks — which is the property the ordering actually carries, and the
	// shape a wedged `podman load` on the far side of an exec presents.
	save := exec.Command("/nonexistent/definitely-not-a-binary")
	load := exec.Command("sh", "-c", "while :; do sleep 0.2; done < /dev/null")

	// Own process group so a kill reaches the fixture AND its `sleep` children, and
	// CommandContext so the kill is CONDITIONAL. An unconditional t.Cleanup kill was wrong
	// twice over: on the passing path StreamLoad has already Killed and Waited this child,
	// so the pid is FREE — signalling it is an unscoped kill by a stale identifier, exactly
	// what the comment further down forbids, widened from one process to a group. The
	// stdlib invokes Cancel only while the process is still running, which is precisely the
	// timeout path this cleanup exists for; it also keeps the Process read inside exec's own
	// synchronisation instead of racing the goroutine below.
	ctx, cancelLoad := context.WithCancel(context.Background())
	load = exec.CommandContext(ctx, load.Path, load.Args[1:]...)
	load.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	load.Cancel = func() error { return syscall.Kill(-load.Process.Pid, syscall.SIGKILL) }

	// StreamLoad runs in a goroutine and the deadline is a select, NOT a time.Since check
	// after the call. Under the reversed order StreamLoad NEVER RETURNS, so a post-hoc
	// elapsed test is unreachable code: the previous version's message could not print, and
	// the run died on `go test`'s default ten-minute timeout with a goroutine dump instead
	// of failing in ten seconds with the sentence written for it. A guard that cannot fire,
	// inside the test written to close a guard that could not fail.
	done := make(chan error, 1)
	go func() { done <- StreamLoad(save, load) }()

	// The cleanup must satisfy BOTH properties, and the previous two rounds each held one
	// and broke the other: round 4's kill was reliable but aimed at a stale identifier;
	// round 5's aimed correctly but only SIGNALLED — cancel closes Done and returns, the
	// kill lands in exec's watcher goroutine, and the test binary can reach os.Exit first.
	// Measured then: five of six failing runs leaked an immortal shell.
	//
	// Synchronous confirmation without reading load.Process (which is what raced): killing
	// the group makes StreamLoad's own load.Wait() return, so the goroutine below sends on
	// `done`. DRAINING that channel IS the proof the child is reaped — by the very call
	// whose absence this test exists to detect.
	// `returned` means StreamLoad RETURNED — not that the child is gone. Conflating the two
	// is what leaked: with the reap removed entirely StreamLoad returns at once, so an early
	// exit on that flag left the fixture alive (measured: 1 leak in 3 runs). The flag only
	// tells us whether draining `done` is still needed for the happens-before edge; the
	// liveness poll below is what actually confirms.
	returned := false
	t.Cleanup(func() {
		cancelLoad()
		if !returned {
			select {
			case <-done: // the group kill let StreamLoad's Wait return
			case <-time.After(10 * time.Second):
				t.Errorf("StreamLoad still blocked 10s after cancel")
				return
			}
		}
		// Past a receive from `done` the goroutine is finished, so reading load.Process here
		// carries a happens-before edge and does not race Start's write.
		if load.Process == nil {
			return
		}
		pid := load.Process.Pid
		deadline := time.Now().Add(10 * time.Second)
		for {
			if err := syscall.Kill(pid, 0); err != nil {
				return // ESRCH: genuinely gone, confirmed rather than assumed
			}
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			if time.Now().After(deadline) {
				t.Errorf("fixture pid %d still alive 10s after cancel: this cleanup returned "+
					"before the child was gone, so a failing run leaks an immortal shell", pid)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})

	var err error
	select {
	case err = <-done:
		returned = true
	case <-time.After(10 * time.Second):
		t.Fatal("StreamLoad did not return within 10s: the load side was Waited on before " +
			"being Killed, so it blocked on a process nothing will ever end")
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
