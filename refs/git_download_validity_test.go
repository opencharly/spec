package refs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitClientDownloadRefusesDeadCachePath is the content-validity gate for the download memo
// (RCA 2026-09-06 — the same class the materialized-tree cache fixed with component drift
// detection): a memoised PATH is only valid while its export EXISTS. A wiped or evicted
// repo-cache dir must NOT be served — the downloader repopulates it on the next call.
func TestGitClientDownloadRefusesDeadCachePath(t *testing.T) {
	dir := t.TempDir()
	client := NewGitClient(filepath.Join(dir, "charly.yml"))

	// Materialize a "downloaded" export + memoise its path.
	export := filepath.Join(dir, "export")
	if err := os.MkdirAll(export, 0o755); err != nil {
		t.Fatalf("create export: %v", err)
	}
	// A live memo entry must also be a CERTIFIED export (v2 provenance, no declared
	// submodules) — path presence alone is not validity (see the uncertified arm below).
	if err := WriteRepoCacheProvenance(export, "0123456789012345678901234567890123456789"); err != nil {
		t.Fatalf("write provenance: %v", err)
	}
	client.mu.Lock()
	client.downloads["github.com/opencharly/example@v2026.240.0001"] = export
	client.mu.Unlock()

	calls := 0
	downloader := func(repoPath, version string) (string, error) {
		calls++
		return export, nil
	}

	got, err := client.Download("github.com/opencharly/example", "v2026.240.0001", downloader)
	if err != nil || got != export || calls != 0 {
		t.Fatalf("a LIVE, CERTIFIED memo entry must be served without re-download (got=%q err=%v calls=%d)", got, err, calls)
	}

	// Evict the export (the repo-cache wipe): the memoised path is now DEAD.
	if err := os.RemoveAll(export); err != nil {
		t.Fatalf("evict export: %v", err)
	}
	got2, err := client.Download("github.com/opencharly/example", "v2026.240.0001", downloader)
	if err != nil || got2 != export || calls != 1 {
		t.Fatalf("a DEAD memo path must re-download (got=%q err=%v calls=%d, want calls=1)", got2, err, calls)
	}
}

// TestGitClientDownloadRefusesUncertifiedExport is the CONTENT gate on the download memo — the
// same predicate the immutable-ref fast path (IsRepoCached) already used.
//
// The defect it closes, measured live on 2026-10-07 against a disposable project pinning a MUTABLE
// `:main` ref: with a persisted resolution present, an export that had LOST its provenance sidecar
// was served for a whole TTL with ZERO upstream contact (0 `git ls-remote` calls; the `.ref`
// sidecar was never restored). `repoCacheFresh` — which the downloader runs — refuses exactly that
// export, but the resolution cache's `os.Stat().IsDir()` answered first and won.
//
// This test FAILS on that implementation: the uncertified arm would be served with zero
// re-downloads.
func TestGitClientDownloadRefusesUncertifiedExport(t *testing.T) {
	dir := t.TempDir()
	client := NewGitClient(filepath.Join(dir, "charly.yml"))

	// An export that CANNOT certify what it holds: the directory exists, the v2 provenance
	// sidecar does not (a wiped cache, a pre-v2 legacy export, or a half-published tree).
	uncertified := filepath.Join(dir, "export-uncertified")
	if err := os.MkdirAll(uncertified, 0o755); err != nil {
		t.Fatalf("create export: %v", err)
	}
	if _, ok := ReadRepoCacheProvenance(uncertified); ok {
		t.Fatalf("fixture must not carry provenance")
	}

	client.mu.Lock()
	client.downloads["github.com/opencharly/example@main"] = uncertified
	client.mu.Unlock()

	calls := 0
	fresh := filepath.Join(dir, "export-fresh")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatalf("create fresh export: %v", err)
	}
	downloader := func(repoPath, version string) (string, error) {
		calls++
		return fresh, nil
	}

	got, err := client.Download("github.com/opencharly/example", "main", downloader)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if calls != 1 {
		t.Fatalf("an UNCERTIFIED export must NOT be served from the memo: the downloader must run "+
			"(calls=%d, want 1) — a path that exists is not a path whose content is certified", calls)
	}
	if got != fresh {
		t.Fatalf("Download returned %q, want the freshly downloaded %q", got, fresh)
	}
}

// TestGitClientDownloadIsNotPersisted is the R7 gate on the RULED design: a resolved ref's DURABLE
// record is the export's own v2 provenance sidecar, never a persisted path beside a timestamp.
//
// Before the cutover a resolution persisted in `cache: git: downloads:` answered "is this ref
// resolved?" for a whole TTL without contacting upstream, so a mutable pin that moved was served
// stale (opencharly/charly#715, #530). A FRESH client (a new process) must therefore ALWAYS
// delegate; only the in-process memo may skip the downloader. This test fails if the resolution is
// persisted again.
func TestGitClientDownloadIsNotPersisted(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "charly.yml")
	client := NewGitClient(cacheFile)

	export := filepath.Join(dir, "export")
	if err := os.MkdirAll(export, 0o755); err != nil {
		t.Fatalf("create export: %v", err)
	}
	if err := WriteRepoCacheProvenance(export, "0123456789012345678901234567890123456789"); err != nil {
		t.Fatalf("write provenance: %v", err)
	}

	calls := 0
	downloader := func(repoPath, version string) (string, error) {
		calls++
		return export, nil
	}

	// First client: one miss, then the in-process memo answers — the batch dedupe.
	if _, err := client.Download("github.com/opencharly/example", "main", downloader); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if _, err := client.Download("github.com/opencharly/example", "main", downloader); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if calls != 1 {
		t.Fatalf("the in-process memo must dedupe the batch (calls=%d, want 1)", calls)
	}

	// A FRESH client is a NEW process: it must re-take the answer from upstream, because nothing
	// durable may claim a resolution it did not take.
	client2 := NewGitClient(cacheFile)
	if _, err := client2.Download("github.com/opencharly/example", "main", downloader); err != nil {
		t.Fatalf("Download (fresh client): %v", err)
	}
	if calls != 2 {
		t.Fatalf("a fresh client must resolve the ref AGAIN (calls=%d, want 2): persisting a "+
			"resolution lets a moved mutable pin be served stale for the whole window — "+
			"opencharly/charly#715", calls)
	}

	// And no durable resolution was written: the per-host section declares no `downloads` key.
	data, err := os.ReadFile(cacheFile)
	if err == nil && strings.Contains(string(data), "downloads") {
		t.Fatalf("the per-host cache still persists a `downloads` resolution:\n%s", data)
	}
}
