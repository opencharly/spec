package spec

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPodLifecycleReplyCarriesServiceOutput pins the op="service" half of #PodLifecycleReply's
// contract: `charly service status` can only tell a finished run-to-completion service from a
// dead one by reading what the init tool printed, so the reply must carry that text. Without the
// `output` field this test does not build — which is the failure the field exists to prevent
// (opencharly/charly#1271).
func TestPodLifecycleReplyCarriesServiceOutput(t *testing.T) {
	const wire = `{"exit_code":3,"output":"immich-db-init                   EXITED    Nov 24 03:13 PM\n"}`

	var got PodLifecycleReply
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ExitCode != 3 {
		t.Fatalf("exit_code = %d, want 3", got.ExitCode)
	}
	if !strings.Contains(got.Output, "immich-db-init") {
		t.Fatalf("output = %q, want the init tool's line", got.Output)
	}

	back, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(back), `"output":"immich-db-init`) {
		t.Fatalf("round trip dropped the output: %s", back)
	}
}
