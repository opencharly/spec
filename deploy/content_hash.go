package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// dirContentHash returns a deterministic content address for a candy's source tree: a sha256 over
// the sorted relative paths, each regular file's executable bit and bytes (symlinks contribute their
// target). It is used to name the build staging directory by CONTENT rather than by ref, so
// the same candy content at two refs produces one COPY source and the BuildKit layer key stays
// stable across a ref-only change.
//
// Volatile metadata is deliberately excluded: mtimes, owners and the `.git` directory differ
// between checkouts of identical content and would defeat the purpose. A read error is returned so
// the caller can fall back to the version-keyed name rather than stage under a wrong hash.
func dirContentHash(root string) (string, error) {
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" || filepath.Base(p) == ".git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	// write hashes one part, turning the (always-succeeding in practice) hash write into a
	// checked one so a real error surfaces instead of being ignored.
	write := func(s string) error {
		_, err := io.WriteString(h, s)
		return err
	}
	for _, rel := range files {
		p := filepath.Join(root, rel)
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		// Only the EXECUTABLE BIT is content. Git applies the process umask to the other
		// permission bits, so two checkouts of identical content can differ there (0644 vs 0664,
		// 0755 vs 0775) and would hash differently — the exact layer-invalidating outcome this
		// exists to prevent. A directory's mode is irrelevant to the staged content (COPY creates
		// the directory), so it contributes nothing.
		executable := "-"
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			if info.Mode()&0o111 != 0 {
				executable = "x"
			}
		}
		for _, part := range []string{rel, "\x00", executable, "\x00"} {
			if err := write(part); err != nil {
				return "", err
			}
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return "", err
			}
			if err := write(target); err != nil {
				return "", err
			}
		case info.Mode().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			_, cerr := io.Copy(h, f)
			closeErr := f.Close()
			if cerr != nil {
				return "", cerr
			}
			if closeErr != nil {
				return "", closeErr
			}
		}
		if err := write("\x00"); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
