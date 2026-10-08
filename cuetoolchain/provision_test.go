package cuetoolchain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tgz builds a release tarball in memory: the `cue` member plus a decoy, so the
// extractor's member SELECTION is exercised rather than assumed.
func tgz(t *testing.T, member string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"README.md": "not the cli", member: "#!/bin/sh\necho cue version v0.16.1\n"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestExtractMemberWritesTheCliAtomically: only the `cue` member lands at dest, it
// is executable, and no .tmp file is left behind (the write is tmp + rename, so an
// interrupted extraction cannot leave a partial binary in place).
func TestExtractMemberWritesTheCliAtomically(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "cue")
	if err := ExtractMember(tgz(t, "cue"), dest); err != nil {
		t.Fatalf("ExtractMember: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "cue version") {
		t.Fatalf("dest does not carry the %q member: %q", Member, got)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("dest mode %v is not executable", fi.Mode().Perm())
	}
	if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("a .tmp file survived the extraction")
	}
}

// TestExtractMemberErrorsOnAMissingMember: a tarball without the CLI must say so.
func TestExtractMemberErrorsOnAMissingMember(t *testing.T) {
	err := ExtractMember(tgz(t, "other"), filepath.Join(t.TempDir(), "cue"))
	if err == nil || !strings.Contains(err.Error(), "no \"cue\" member") {
		t.Fatalf("want a missing-member error, got %v", err)
	}
}

// TestFetchRefusesAChecksumMismatch: the verification is the point of the pin, so
// a mismatch must fail BEFORE anything is written — and leave nothing behind.
func TestFetchRefusesAChecksumMismatch(t *testing.T) {
	tarball := tgz(t, "cue")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "cue")
	err := fetch(srv.URL, strings.Repeat("0", 64), dest)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want a checksum-mismatch error, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("a refused download left a binary at dest")
	}
}

// TestFetchAcceptsThePinnedChecksum is the other half: with the REAL checksum of
// what the server serves, the member is written and runnable.
func TestFetchAcceptsThePinnedChecksum(t *testing.T) {
	tarball := tgz(t, "cue")
	sum := sha256.Sum256(tarball)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "cue")
	if err := fetch(srv.URL, hex.EncodeToString(sum[:]), dest); err != nil {
		t.Fatalf("fetch with the correct checksum failed: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("the member was not written: %v", err)
	}
}

// TestEnsureAcceptsAPinnedBinaryWithoutDownloading: an already-pinned binary is
// reused, which is what makes the provisioning idempotent.
func TestEnsureAcceptsAPinnedBinaryWithoutDownloading(t *testing.T) {
	dir := t.TempDir()
	writeFakeCue(t, filepath.Join(dir, Member), "cue version "+Version)
	got, err := Ensure(dir)
	if err != nil {
		t.Fatalf("Ensure on a pinned binary: %v", err)
	}
	if got != filepath.Join(dir, Member) {
		t.Fatalf("Ensure returned %q, want the existing %q", got, filepath.Join(dir, Member))
	}
}

// TestEnsureNeverServesAStaleCache: a binary reporting another version must NOT be
// returned as success. In a hermetic environment Ensure then fails (with the
// remedy named); where the release is reachable it provisions the PINNED one — the
// assertion is that whatever comes back is the pin, and never the stale path.
func TestEnsureNeverServesAStaleCache(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, Member)
	writeFakeCue(t, stale, "cue version v0.0.1-not-the-pin")
	got, err := Ensure(dir)
	if err != nil {
		if !strings.Contains(err.Error(), Version) {
			t.Fatalf("the failure must name the pin, got: %v", err)
		}
		return // hermetic: the download could not complete, and the stale binary was refused
	}
	// NOTE: `got` is the SAME PATH as the stale file by design — dest is where the pin
	// lives, and re-provisioning overwrites it. So the assertion cannot be path
	// inequality (my first version asserted that and was wrong): it is that whatever
	// sits at the returned path now REPORTS the pinned version.
	if v, verr := version(got); verr != nil || v != Version {
		t.Fatalf("Ensure returned %q which reports %q (err %v) — the stale binary was not replaced, want %s", got, v, verr, Version)
	}
}

func writeFakeCue(t *testing.T, path, versionLine string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho "+versionLine+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
