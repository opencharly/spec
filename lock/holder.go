package lock

// holder.go — WHO holds a contended flock, discovered from the KERNEL rather than from the lock
// file's bytes.
//
// A wait that ends in failure is only actionable if it can NAME the process being waited on, and
// the lock file is deliberately content-free (see AcquireFileLock: the pid line was deleted because
// nothing read it and it implied a staleness mechanism this lock does not have). The kernel
// already publishes the answer: /proc/<pid>/fdinfo/<fd> carries the file's inode AND every lock
// held through that descriptor — `lock:\t1: FLOCK  ADVISORY  WRITE <pid> <maj:min:ino> 0 EOF`. A
// waiter matches the lock file's inode, resolves the owning pid, and reads that pid's command line.
// No lock-file bytes, no pidfile illusion, and the answer is always CURRENT: a dead holder cannot
// appear, because the kernel drops its flock on exit. A waiter that merely OPENED the file (not yet
// holding it) has no `lock:` line and is therefore never mistaken for the holder.
//
// Best-effort by design: on a kernel without fdinfo locks, or a platform without /proc, the
// description degrades to "another process (holder not identifiable on this platform)". The WAIT
// itself is unaffected — this only changes the report.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// holderDescription names the process currently holding the flock on path, for a waiter's progress
// report and timeout error. Never returns an empty string.
func holderDescription(path string) string {
	pids := flockHolderPIDs(path)
	if len(pids) == 0 {
		return "another process (holder not identifiable on this platform)"
	}
	names := make([]string, 0, len(pids))
	for _, pid := range pids {
		names = append(names, fmt.Sprintf("pid %d (%s)", pid, procCmdline(pid)))
	}
	return strings.Join(names, ", ")
}

// flockHolderPIDs returns the pids that hold an flock on path, matched by the lock file's inode via
// /proc/<pid>/fdinfo/<fd>. Empty when the platform/kernel cannot answer (never an error: this is a
// diagnostic, and a waiter must not fail differently because a diagnostic was unavailable).
func flockHolderPIDs(path string) []int {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	// fmt rather than strconv: Stat_t.Ino is uint64 on linux/amd64 but uint32 on the 32-bit ports,
	// and the explicit uint64() conversion is a lint finding (unconvert) on the platform CI runs.
	inoLine := fmt.Sprintf("ino:\t%d", st.Ino)
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue // not a pid dir
		}
		fdinfoDir := filepath.Join("/proc", p.Name(), "fdinfo")
		fds, err := os.ReadDir(fdinfoDir)
		if err != nil {
			continue // not a process we can inspect, or it exited
		}
		for _, fd := range fds {
			b, err := os.ReadFile(filepath.Join(fdinfoDir, fd.Name()))
			if err != nil {
				continue // raced with exit
			}
			s := string(b)
			if !strings.Contains(s, inoLine) || !strings.Contains(s, "lock:") {
				continue
			}
			pids = append(pids, pid)
			break
		}
	}
	return pids
}

// procCmdline renders pid's command line (best-effort, bounded so one lock report cannot flood a
// log with a pathological argv).
func procCmdline(pid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(b) == 0 {
		return "command line unavailable"
	}
	s := strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", " "))
	if s == "" {
		return "command line empty (kernel thread?)"
	}
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
