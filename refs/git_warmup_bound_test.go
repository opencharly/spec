package refs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// warmUpLog opens the progress sink the phase writes to; warmUpLogContent reads it back.
func warmUpLog(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "warmup-progress")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func warmUpLogContent(t *testing.T, f *os.File) string {
	t.Helper()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// warmUpFixture points every network seam in the prefetch pool at a controllable stub and returns
// a restore func. Both bounds are shortened so a mutation is a REAL failure: with the production
// 30s/60s git-op bounds left in place, a "the bound fired" assertion could pass while proving
// nothing.
func warmUpFixture(t *testing.T, perFetch time.Duration) (restore func()) {
	t.Helper()
	origFetch, origBranch := gitLatestTagFetch, gitDefaultBranchFn
	origTotal := warmUpTotalTimeout
	origMeta, origTransfer := gitOpMetadataTimeout, gitOpTransferTimeout
	gitLatestTagFetch = func(string) (string, error) {
		time.Sleep(perFetch)
		return "v2026.240.0001", nil
	}
	gitDefaultBranchFn = func(string) (string, error) {
		time.Sleep(perFetch)
		return "main", nil
	}
	// Shorten the git-op bounds too: the pool must be stopped by the PHASE bound, not by a
	// per-fetch bound that happens to be shorter.
	gitOpMetadataTimeout = 5 * time.Second
	gitOpTransferTimeout = 5 * time.Second
	return func() {
		gitLatestTagFetch, gitDefaultBranchFn = origFetch, origBranch
		warmUpTotalTimeout = origTotal
		gitOpMetadataTimeout, gitOpTransferTimeout = origMeta, origTransfer
	}
}

// TestWarmUp_TotalBoundStopsThePrefetch is the R7 gate on the phase's TOTAL ceiling. A per-fetch
// bound does not bound a pool: with 40 repos, 10 workers and a 50ms fetch, an unbounded pool runs
// ~200ms and, on a real 194-repo corpus, N/workers x per-fetch-bound. Without warmUpTotalTimeout
// this test runs to completion (~200ms) and fails its elapsed assertion.
func TestWarmUp_TotalBoundStopsThePrefetch(t *testing.T) {
	restore := warmUpFixture(t, 50*time.Millisecond)
	defer restore()
	warmUpTotalTimeout = 60 * time.Millisecond

	dir := t.TempDir()
	g := NewGitClient(filepath.Join(dir, "charly.yml"))

	var repos []string
	for i := 0; i < 40; i++ {
		repos = append(repos, "https://github.com/opencharly/repo-"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	log := warmUpLog(t)
	start := time.Now()
	g.WarmUp(repos, log)
	elapsed := time.Since(start)
	out := warmUpLogContent(t, log)

	if elapsed > 400*time.Millisecond {
		t.Fatalf("WarmUp ran %s for %d repos with a 60ms phase bound — the phase has no TOTAL "+
			"ceiling, so its runtime is (len(repos)/workers) x per-fetch-bound, unbounded in the "+
			"corpus size (charly#650's resolution stall)", elapsed, len(repos))
	}
	if !strings.Contains(out, "bound") || !strings.Contains(out, "on demand") {
		t.Fatalf("a phase cut off by its bound must SAY so (progress, not silence): %q", out)
	}
}

// TestWarmUp_WritesTheCacheOnce — the write coalescing. Each LatestTag/DefaultBranch would
// otherwise pay a full per-host YAML read-modify-write under an advisory file lock; the batch must
// defer them to ONE write. Fails (with one write per answer) if the suppression is removed.
func TestWarmUp_WritesTheCacheOnce(t *testing.T) {
	restore := warmUpFixture(t, time.Millisecond)
	defer restore()

	var saves int
	origSaveFn := warmUpSaveFn
	warmUpSaveFn = func() { saves++ }
	defer func() { warmUpSaveFn = origSaveFn }()

	dir := t.TempDir()
	g := NewGitClient(filepath.Join(dir, "charly.yml"))

	repos := []string{"https://github.com/opencharly/a", "https://github.com/opencharly/b", "https://github.com/opencharly/c"}
	g.WarmUp(repos, warmUpLog(t))

	if saves != 1 {
		t.Fatalf("WarmUp wrote the per-host cache %d time(s) for %d repos — the batch must coalesce "+
			"its writes to ONE (a full YAML read-modify-write per answer is ~2 per repo)", saves, len(repos))
	}
}

// TestWarmUp_ReportsProgress — a bounded phase must be distinguishable from a hang in the log.
func TestWarmUp_ReportsProgress(t *testing.T) {
	restore := warmUpFixture(t, time.Millisecond)
	defer restore()

	dir := t.TempDir()
	g := NewGitClient(filepath.Join(dir, "charly.yml"))
	log := warmUpLog(t)
	g.WarmUp([]string{"https://github.com/opencharly/a", "https://github.com/opencharly/b"}, log)
	out := warmUpLogContent(t, log)
	if !strings.Contains(out, "fetching git metadata for 2 repo(s)") {
		t.Fatalf("the phase must announce what it is about to do: %q", out)
	}
	if !strings.Contains(out, "cached (2 repo(s))") {
		t.Fatalf("the phase must report how many repos it WARMED, so a partial run is visible: %q", out)
	}
}
