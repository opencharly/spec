package container

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStreamLoad_PipesPayload asserts the primitive actually MOVES the bytes — the
// property every image-delivery path in the tree rests on. It discriminates: a
// StreamLoad that started both commands but never wired the pipe would leave the
// destination file empty and fail here.
func TestStreamLoad_PipesPayload(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	const payload = "layer-blob-bytes"

	save := exec.Command("printf", "%s", payload)
	load := exec.Command("sh", "-c", "cat > "+dst)

	if err := StreamLoad(save, load); err != nil {
		t.Fatalf("StreamLoad: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading destination: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("destination = %q, want %q", got, payload)
	}
}

// TestStreamLoad_ReportsSaveFailure asserts a failing SAVE side is surfaced rather
// than masked by a load side that exits 0 regardless. This is the real hazard: the
// load process happily reports success on a short stream, so a caller that only
// watched the load exit code would call a truncated transfer a success.
func TestStreamLoad_ReportsSaveFailure(t *testing.T) {
	save := exec.Command("sh", "-c", "exit 3")
	load := exec.Command("cat")

	err := StreamLoad(save, load)
	if err == nil {
		t.Fatal("StreamLoad returned nil for a failing save side")
	}
	if !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("error %q does not name the save side", err)
	}
}

// TestStreamLoad_ReportsLoadFailure asserts a failing LOAD side is surfaced even
// when the save side succeeds.
func TestStreamLoad_ReportsLoadFailure(t *testing.T) {
	save := exec.Command("printf", "x")
	load := exec.Command("sh", "-c", "cat >/dev/null; exit 4")

	err := StreamLoad(save, load)
	if err == nil {
		t.Fatal("StreamLoad returned nil for a failing load side")
	}
	if !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("error %q does not name the load side", err)
	}
}
