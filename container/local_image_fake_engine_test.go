package container

// local_image_fake_engine_test.go — the ONE fake-engine seam for this package's tests.
//
// Every test of `defaultListLocalImages` needs the same thing: an `exec.Command(<engine>,
// ...)` that it controls. Rather than each test writing its own script into its own temp
// dir and prepending it to PATH (which is how two copies of that setup came to exist), the
// helper lives here once and the tests differ only in the script they hand it.

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeEngine writes `script` as an executable named after `name` in a fresh temp dir and
// puts that dir first on PATH, so `exec.Command(name, ...)` resolves to it. The temp dir is
// t.TempDir(), so it is cleaned up with the test.
//
// The seam is deliberately the smallest one that is real: the child process genuinely runs,
// genuinely writes to its stderr, and genuinely exits with the status the script chooses —
// only the engine is simulated, never the code under test.
func fakeEngine(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
