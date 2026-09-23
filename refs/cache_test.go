package refs

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsRepoCached_IncompleteExportIsNotCached is the IMMUTABLE-ref half of the
// self-heal contract, and it needs its own test because repoCacheFresh never
// sees a tag: EnsureRepoDownloaded short-circuits on
// `cached && !IsMutableRef(version)` and returns RepoCachePath without entering
// DownloadRepo. So directory-exists alone pinned every incomplete TAG export
// permanently — a tag never moves, so nothing would re-fetch it. Found live:
// charly@v2026.183.1359 held 0 of 9 declared submodules and charly@v2026.201.0706
// held 1 of 10.
func TestIsRepoCached_IncompleteExportIsNotCached(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("CHARLY_REPO_CACHE", cacheDir)

	const repo = "github.com/opencharly/charly"
	mk := func(t *testing.T, version string, populate bool) {
		t.Helper()
		cache := filepath.Join(cacheDir, repo+"@"+version)
		if err := os.MkdirAll(filepath.Join(cache, "spec"), 0o755); err != nil {
			t.Fatal(err)
		}
		if populate {
			if err := os.WriteFile(filepath.Join(cache, "spec", "go.mod"), []byte("module y\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(cache, ".gitmodules"),
			[]byte("[submodule \"spec\"]\n\tpath = spec\n\turl = https://example.invalid/spec.git\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// V2 provenance: the tree is certifiably pristine.
		if err := WriteRepoCacheProvenance(cache, version); err != nil {
			t.Fatal(err)
		}
	}

	mk(t, "v1.0.0", false) // the shape the old fetch left behind
	got, err := IsRepoCached(repo, "v1.0.0")
	if err != nil {
		t.Fatalf("IsRepoCached: %v", err)
	}
	if got {
		t.Error("an incomplete TAG export must NOT count as cached — it can never be " +
			"re-fetched otherwise, since a tag does not move and repoCacheFresh never sees it")
	}

	mk(t, "v2.0.0", true) // complete: must stay a hit, or every access re-clones
	got, err = IsRepoCached(repo, "v2.0.0")
	if err != nil {
		t.Fatalf("IsRepoCached: %v", err)
	}
	if !got {
		t.Error("a COMPLETE export must remain cached")
	}

	// Absent entirely: not cached, and not an error.
	got, err = IsRepoCached(repo, "v9.9.9")
	if err != nil {
		t.Fatalf("IsRepoCached on a missing entry must not error: %v", err)
	}
	if got {
		t.Error("a missing cache entry must not report cached")
	}
}

// TestIsRepoCached_LegacyProvenanceIsNotCached locks the v2 provenance gate: a
// LEGACY (v1: bare-commit) sidecar cannot certify its tree — the pre-cutover
// loader could rewrite an export in place — so the export must be treated as NOT
// cached, forcing one pristine re-fetch. Without this the polluted cache stays a
// permanent hit for immutable refs (the exact bug this cutover fixes).
func TestIsRepoCached_LegacyProvenanceIsNotCached(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("CHARLY_REPO_CACHE", cacheDir)

	const repo = "github.com/opencharly/charly"
	cache := filepath.Join(cacheDir, repo+"@main")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	// The legacy sidecar: just the commit hash, no version envelope.
	if err := os.WriteFile(RefProvenancePath(cache), []byte("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := IsRepoCached(repo, "main")
	if err != nil {
		t.Fatalf("IsRepoCached: %v", err)
	}
	if got {
		t.Fatal("a legacy (v1) provenance sidecar must NOT count as cached — it cannot " +
			"certify its content, so the export must be re-fetched pristine")
	}
	if _, ok := ReadRepoCacheProvenance(cache); ok {
		t.Fatal("ReadRepoCacheProvenance must reject the legacy bare-commit format")
	}
}
