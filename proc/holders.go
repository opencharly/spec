package proc

import (
	"os"
	"path/filepath"
	"strconv"
)

// holders.go — "which processes hold this file open?", the kernel-backed answer.
//
// This exists because the alternatives are all wrong in ways that look right:
//
//   - A PIDFILE is wrong. spec/lock deliberately writes NOTHING into its lock files,
//     because the kernel releases an flock when the holder dies: the file's presence
//     proves nothing and its absence proves nothing. A `pid=` line there previously
//     misled three separate readers into trusting the file instead of the process
//     table, which is why it was removed.
//   - Matching by process NAME is wrong. `pkill -x charly` matches `comm` exactly and
//     is blind to cwd, session, systemd scope and cgroup, so it cannot distinguish one
//     session's run from every other session's on the same host. That is not a flag
//     that was missed — it is unscoped by construction.
//
// What IS sound is asking the kernel who currently has the file open, which is what
// fuser(1) does: walk /proc/<pid>/fd and compare by INODE, not by path string, so a
// relative path, a symlink, or a bind mount cannot produce a false negative.
//
// Linux-specific by nature (/proc), and deliberately so — it degrades to "nobody holds
// it" rather than erroring on a platform without /proc, which is the safe direction for
// every caller: a stop verb that finds no holder does nothing.

// PIDsHoldingPath returns the PIDs of every process holding path open, excluding the
// calling process. An empty result means no live holder — which, for a lock file, is
// the only trustworthy way to say "no run is in flight".
//
// Processes owned by other users are skipped silently: their /proc/<pid>/fd is not
// readable, and a caller that cannot signal them has no use for their PIDs anyway.
func PIDsHoldingPath(path string) []int {
	target, err := os.Stat(path)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var pids []int
	for _, e := range entries {
		if !e.IsDir() || !allDigits(e.Name()) {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if pid <= 0 || pid == self {
			continue
		}
		if processHoldsFile(pid, target) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// processHoldsFile reports whether pid has target open on any descriptor. The
// comparison is os.SameFile — device+inode — so it is immune to the path-spelling
// differences that defeat a string match.
func processHoldsFile(pid int, target os.FileInfo) bool {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, fd := range fds {
		fi, err := os.Stat(filepath.Join(fdDir, fd.Name()))
		if err != nil {
			continue
		}
		if os.SameFile(fi, target) {
			return true
		}
	}
	return false
}
