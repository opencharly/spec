// Package perhost is THE single reader and writer of the per-host charly config
// (~/.config/charly/charly.yml, honoring CHARLY_DEPLOY_CONFIG).
//
// The file is a SHARED document: the deploy overlay, the git-ref cache, the install ledger and the
// system info all live in it as top-level sections, written by DIFFERENT charly commands that can
// run CONCURRENTLY. Four independent copies of "take <path>.lock, read the yaml.Node, find-or-create
// my key, marshal, tempfile+rename" grew here, and the copies disagreed about the contract in ways
// that destroyed real data on 2026-11-24:
//
//   - one cleared a section by DELETING the whole file (every deploy entry with it);
//   - one wrote a stale in-memory value over a value another process had just changed.
//
// Update is that contract, stated once: one lock, one read INSIDE the lock, one mutate that touches
// only the keys it owns, one atomic rename. A corrupt or unparseable file is never clobbered —
// mutate is not called at all. Callers supply ONLY the section logic; the file mechanics are here.
package perhost

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opencharly/spec/lock"
	"gopkg.in/yaml.v3"
)

// LockWait bounds the wait for the shared advisory lock. A section write is milliseconds, so a
// legitimate queue clears well inside this; a holder that outlives it is a stuck writer or a
// cross-command collision, and both deserve a fast, named error rather than an unbounded hang.
const LockWait = 10 * time.Second

// Read parses the per-host config at path. A missing file is (nil, nil) — a fresh host; a corrupt
// file is an error, never an empty document (a writer must refuse, not truncate).
func Read(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc yaml.Node
	if uerr := yaml.Unmarshal(data, &doc); uerr != nil {
		return nil, fmt.Errorf("per-host config %s does not parse: %w", path, uerr)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}
	return &doc, nil
}

// Update runs ONE locked read-modify-write cycle over the per-host config:
//
//  1. take <path>.lock (bounded: LockWait);
//  2. Read the CURRENT on-disk document — never a caller's snapshot, which is how a concurrent
//     writer's change gets lost;
//  3. call mutate on the document's root mapping; mutate touches ONLY the keys it owns and
//     reports whether anything changed;
//  4. when it did, marshal and replace the file atomically (tempfile in the same directory +
//     rename), so a reader never sees a half-written file.
//
// A corrupt file is an error and mutate is NOT called: the bytes stay for recovery.
func Update(path string, mutate func(root *yaml.Node) (changed bool, err error)) error {
	if mutate == nil {
		return fmt.Errorf("perhost.Update: mutate is nil")
	}
	release, err := lock.AcquireFileLockWithin(path+".lock", true, LockWait)
	if err != nil {
		return fmt.Errorf("per-host config %s: %w", path, err)
	}
	defer func() { _ = release() }()

	doc, err := Read(path)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
			{Kind: yaml.MappingNode, Tag: "!!map"},
		}}
	}
	root := doc.Content[0]
	if root == nil || root.Kind != yaml.MappingNode {
		return fmt.Errorf("per-host config %s: the document root is not a mapping", path)
	}
	changed, err := mutate(root)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("per-host config %s: marshal: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("per-host config %s: mkdir: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".charly-perhost-*")
	if err != nil {
		return fmt.Errorf("per-host config %s: tempfile: %w", path, err)
	}
	name := tmp.Name()
	if _, werr := tmp.Write(out); werr != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("per-host config %s: write: %w", path, werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("per-host config %s: close: %w", path, cerr)
	}
	if merr := os.Chmod(name, 0o600); merr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("per-host config %s: chmod: %w", path, merr)
	}
	if rerr := os.Rename(name, path); rerr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("per-host config %s: rename: %w", path, rerr)
	}
	return nil
}

// Get returns the value node at the given key path (e.g. Get(root, "cache", "git", "bypass")), or
// nil when any step is absent or not a mapping.
func Get(root *yaml.Node, keys ...string) *yaml.Node {
	n := root
	for _, k := range keys {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == k {
				next = n.Content[i+1]
				break
			}
		}
		n = next
	}
	return n
}

// Set stores value at the given key path, creating intermediate mappings. A nil value REMOVES the
// leaf key. It reports whether the document changed.
func Set(root *yaml.Node, value *yaml.Node, keys ...string) bool {
	if len(keys) == 0 {
		return false
	}
	parent := root
	for _, k := range keys[:len(keys)-1] {
		child := Get(parent, k)
		if child == nil || child.Kind != yaml.MappingNode {
			if value == nil || parent == nil {
				return false // nothing to remove along a path that does not exist
			}
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, child)
		}
		parent = child
	}
	leaf := keys[len(keys)-1]
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == leaf {
			if value == nil {
				parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
				return true
			}
			parent.Content[i+1] = value
			return true
		}
	}
	if value == nil {
		return false
	}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: leaf}, value)
	return true
}
