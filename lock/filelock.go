// Package lock is the spec fabric slice for advisory file locking — a host primitive a
// plugin physically cannot hold (a syscall.Flock over an on-disk lock file). RELOCATED from
// sdk/kit (#55 fabric-primitive extraction). It carries the heavy syscall dependency in its
// OWN slice (Rule 2: minimal imports) so a consumer that needs a lock never drags anything
// else, and a consumer that needs nothing else never drags syscall. charly core inlines from
// here; sdk/kit re-exports the same symbols so the compiled-in candy/plugin-preempt and other
// plugin call sites are untouched.
package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ErrLockBusy is returned by a NON-blocking AcquireFileLock when another holder already owns the
// lock. Callers detect it with errors.Is to render a precise "already in progress" message.
var ErrLockBusy = errors.New("file lock held by another process")

// lockTimeout bounds the BLOCKING acquire wait. Under heavy concurrent load (parallel bed runs),
// a peer holding the lock can stall; an unbounded flock would hang the caller forever (the
// recurring fleet-del stall). A package var (not a const) so a test can shorten it.
var lockTimeout = 2 * time.Minute

// flockBounded acquires an exclusive flock, failing fast after lockTimeout instead of blocking
// forever on a contended lock.
func flockBounded(f *os.File, path string) error {
	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK {
			return fmt.Errorf("flock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("flock %s: lock held by another process for > %s", path, lockTimeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// AcquireFileLock takes an advisory flock on path (creating the file + parent dirs on demand) and
// returns a release closure that unlocks + closes.
//
// blocking selects the contention behavior:
//   - true  → LOCK_EX: wait until the lock is free, failing fast after lockTimeout (bounded, never hangs).
//   - false → LOCK_EX|LOCK_NB: return ErrLockBusy immediately when another holder exists.
//
// The lock file is deliberately NOT unlinked on release (unlinking a held lock races a waiter
// that already opened the prior inode). flock is per-open-file-description, so two acquires of the
// same path — even within ONE process — contend, which the duplicate-run guard relies on.
func AcquireFileLock(path string, blocking bool) (release func() error, err error) {
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return nil, fmt.Errorf("create lock dir %s: %w", filepath.Dir(path), mkErr)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	if !blocking {
		if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); flockErr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("%s: %w", path, ErrLockBusy)
		}
	} else if flockErr := flockBounded(f, path); flockErr != nil {
		_ = f.Close()
		return nil, flockErr
	}
	// Truncate but write NOTHING. The truncate is deliberate and is not dead code: O_CREATE|O_RDWR
	// does not truncate, so without it a re-acquired lock retains the PREVIOUS holder's bytes —
	// turning an empty file into a stale one, which is worse, because staleness reads as currency.
	//
	// Nothing is written because nothing reads it. This previously wrote "pid=%d", which was
	// decorative for every caller: the ONE caller needing content (candy/plugin-box's
	// build-activity lock) overwrites the file with its build CalVer immediately after acquiring,
	// and the ONE reader (candy/plugin-clean's retention floor) reads only that. For the other
	// twelve ACQUISITION SITES the line was read by nothing — while teaching every human who
	// opened the file that this is a PIDFILE lock whose staleness must be reasoned about.
	//
	// It is not: the kernel releases an flock when the holder dies, so the file's PRESENCE proves
	// nothing and its ABSENCE proves nothing. `ps` is the only discriminator, and the pid line
	// misled three separate readers into reaching for the file instead.
	//
	// "Acquisition sites" is a named framing, chosen on purpose: eleven sites call
	// AcquireFileLock directly, and two more acquire through the wrappers below
	// (AcquireImageBuildLock from candy/plugin-build, AcquireVmDomainLock from
	// candy/plugin-check) without ever naming it, so no grep on this identifier reaches them.
	// A lock taken through a wrapper carried the line just the same, which is why the population
	// this claim is about is acquisitions rather than direct calls.
	_ = f.Truncate(0)
	return func() error {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return f.Close()
	}, nil
}

// ImageBuildLockPath is the pure per-image build-lock key derivation: the
// user-cache lock file for an image ref, with the :tag stripped (preserving any
// registry:port colon) so every CalVer build of one image shares ONE lock — a
// shared intermediate built COLD once while distinct leaves fan out in parallel.
// Shared across the module boundary (R3) so charly core's acquireImageBuildLock
// AND the compiled-in candy/plugin-build DRIVE derive the byte-identical path.
func ImageBuildLockPath(fullTag string) (string, error) {
	ref := fullTag
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i] // strip :<tag>, preserving any registry:port colon
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("image build lock: %w", err)
	}
	dir := filepath.Join(cache, "charly", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("image build lock dir: %w", err)
	}
	sum := sha256.Sum256([]byte(ref))
	return filepath.Join(dir, "image-"+hex.EncodeToString(sum[:8])+".lock"), nil
}

// AcquireImageBuildLock takes the blocking per-image build lock for fullTag.
func AcquireImageBuildLock(fullTag string) (func() error, error) {
	path, err := ImageBuildLockPath(fullTag)
	if err != nil {
		return nil, err
	}
	return AcquireFileLock(path, true)
}

// AcquireLocalPkgBuildLock serializes concurrent localpkg builds sharing a source dir.
func AcquireLocalPkgBuildLock(srcDir string) (func() error, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("localpkg build lock: %w", err)
	}
	dir := filepath.Join(cache, "charly", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("localpkg build lock dir: %w", err)
	}
	sum := sha256.Sum256([]byte(srcDir))
	return AcquireFileLock(filepath.Join(dir, "localpkg-"+hex.EncodeToString(sum[:8])+".lock"), true)
}
