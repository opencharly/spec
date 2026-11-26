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
// the sorted relative paths, each entry's mode, and each regular file's bytes (symlinks contribute
// their target). It is used to name the build staging directory by CONTENT rather than by ref, so
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
	for _, rel := range files {
		p := filepath.Join(root, rel)
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		io.WriteString(h, rel)
		io.WriteString(h, "\x00")
		io.WriteString(h, info.Mode().String())
		io.WriteString(h, "\x00")
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return "", err
			}
			io.WriteString(h, target)
		case info.Mode().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(h, f); err != nil {
				f.Close()
				return "", err
			}
			f.Close()
		}
		io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
