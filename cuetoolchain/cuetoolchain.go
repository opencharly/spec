// Package cuetoolchain is THE single home for the pinned `cue` CLI (R3).
//
// The schema→Go pipeline runs `cue exp gengotypes`, and that generator lives in
// `cuelang.org/go`'s INTERNAL tree (`internal/encoding/gotypes`), so no other
// module can call it in-process. The CLI is therefore a toolchain the pipeline
// provisions itself — checksum-verified, from a pinned release — so a reader who
// is told to run a `charly` command never sees `cue`, a checkout path, or a `cd`.
//
// The pin AND the provisioning live here, once: the `charly`-side generation verb
// calls Ensure to obtain the toolchain, and spec's own `bootstrap-cue` task is
// re-keyed onto it too (it used to carry the same version, URL and checksum inline
// in shell — two declarations of one toolchain, which is exactly what this package
// removes). One pin, one home, one fetcher.
//
// STANDALONE: stdlib only, so both callers can import it without pulling in the
// packages they regenerate.
package cuetoolchain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Version is the pinned release tag of the `cue` CLI.
const Version = "v0.16.1"

// Member is the path of the CLI inside the release tarball.
const Member = "cue"

// sha256ByArch pins the release tarball per GOARCH. Only the arches listed are
// usable: a caller must consult SHA256 and fail LOUDLY on an unpinned arch rather
// than download a binary it cannot verify. Widening this map is a deliberate act
// (a checksum someone verified), never a fallback.
var sha256ByArch = map[string]string{
	"amd64": "5d644c1305a2b86504c8dcd2ec829cf5b4999efc2cf51ee375624e0455f774ae",
}

// Tarball returns the release tarball name for a GOARCH, or "" when that arch is
// not checksum-pinned.
func Tarball(goarch string) string {
	if _, ok := sha256ByArch[goarch]; !ok {
		return ""
	}
	return fmt.Sprintf("cue_%s_linux_%s.tar.gz", Version, goarch)
}

// URL returns the pinned download URL for a GOARCH, or "" when that arch is not
// checksum-pinned.
func URL(goarch string) string {
	t := Tarball(goarch)
	if t == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/cue-lang/cue/releases/download/%s/%s", Version, t)
}

// SHA256 returns the pinned checksum for a GOARCH and whether that arch is
// pinned at all — the check a caller must make BEFORE fetching, so an unpinned
// arch fails with a remedy rather than an unverifiable download.
func SHA256(goarch string) (string, bool) {
	sum, ok := sha256ByArch[goarch]
	return sum, ok
}

// Pin names the pinned toolchain for the running host in one line, for a `--help`
// or a log line: what will be provisioned, and which arch rule applies. It is the
// ONE place a caller takes that sentence from.
func Pin() string {
	return fmt.Sprintf("cue %s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
}

// Ensure returns a path to the pinned cue CLI, provisioning it into dir on first
// use and verifying it on every call. It is the ONE fetcher:
//
//   - an existing binary is accepted only if it REPORTS the pinned version, so a
//     stale cache is re-provisioned rather than served as success;
//   - an unpinned arch fails before any download, naming the arch and the remedy;
//   - the checksum is verified BEFORE extraction, so a mismatch leaves no
//     half-written binary behind (the write is atomic: tmp + rename);
//   - every failure names the pin, the URL and the destination.
func Ensure(dir string) (string, error) {
	dest := filepath.Join(dir, Member)
	if v, err := version(dest); err == nil && v == Version {
		return dest, nil
	}
	sum, ok := SHA256(runtime.GOARCH)
	if !ok {
		return "", fmt.Errorf("the pinned cue %s is not checksum-pinned for %s/%s (only linux/amd64 is) — pin it in github.com/opencharly/spec/cuetoolchain, or run the pipeline in a container",
			Version, runtime.GOOS, runtime.GOARCH)
	}
	url := URL(runtime.GOARCH)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := fetch(url, sum, dest); err != nil {
		return "", fmt.Errorf("provisioning %s from %s failed: %w (retry, or pre-place the binary at %s)", Pin(), url, err, dest)
	}
	v, err := version(dest)
	if err != nil {
		return "", fmt.Errorf("the provisioned binary at %s does not run: %w", dest, err)
	}
	if v != Version {
		return "", fmt.Errorf("provisioned %s reports %q, want %s", dest, v, Version)
	}
	return dest, nil
}

// version runs `<bin> version` and returns the version token it prints.
func version(bin string) (string, error) {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return "", err
	}
	for _, f := range strings.Fields(string(out)) {
		if strings.HasPrefix(f, "v") && strings.Contains(f, ".") {
			return f, nil
		}
	}
	return "", fmt.Errorf("no version token in %q", strings.TrimSpace(string(out)))
}

// fetch downloads url, refuses to extract unless its sha256 matches wantSHA, and
// writes the tarball's Member entry to dest atomically.
func fetch(url, wantSHA, dest string) error {
	resp, err := http.Get(url) //nolint:gosec,noctx // a pinned, checksum-verified release URL
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	tgz, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	got := sha256.Sum256(tgz)
	if hex.EncodeToString(got[:]) != wantSHA {
		return fmt.Errorf("checksum mismatch: got %s, want %s — refusing to extract", hex.EncodeToString(got[:]), wantSHA)
	}
	return ExtractMember(tgz, dest)
}

// ExtractMember writes the release tarball's Member entry to dest atomically
// (tmp + rename), so an interrupted extraction cannot leave a partial binary at
// the destination path.
func ExtractMember(tgz []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("no %q member in the release tarball", Member)
		}
		if err != nil {
			return err
		}
		if filepath.Clean(hdr.Name) != Member {
			continue
		}
		tmp := dest + ".tmp"
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, dest)
	}
}
