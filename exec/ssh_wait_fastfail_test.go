package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/opencharly/spec/poll"
)

// TestWaitForSSH_MissingAliasFailsFast: the check-live hang regression — a
// managed alias that does not resolve ("Could not resolve hostname") is a
// PERMANENT failure; WaitForSSH must abort immediately via ErrPollFatal
// instead of burning the whole readiness cap on a condition that can never
// become ready. The poll here would otherwise run for 30s; the fix must fail
// in well under a second.
func TestWaitForSSH_MissingAliasFailsFast(t *testing.T) {
	ssh := SSHArgs{Host: "charly-definitely-missing-alias", ConnectTimeout: 2}
	start := time.Now()
	err := WaitForSSH(context.Background(), ssh, func(ctx context.Context, cond poll.PollCondition) error {
		// A cap-only poll: retry until the 30s cap. The fix must abort it early.
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			ready, _, cerr := cond(ctx)
			if ready {
				return nil
			}
			if cerr != nil && errors.Is(cerr, poll.ErrPollFatal) {
				return cerr
			}
			time.Sleep(200 * time.Millisecond)
		}
		return poll.ErrPollCapExceeded
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("WaitForSSH(missing alias) = nil, want an error")
	}
	if !errors.Is(err, poll.ErrPollFatal) {
		t.Fatalf("WaitForSSH(missing alias) error = %v, want ErrPollFatal wrapped", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("WaitForSSH(missing alias) took %v — the cap was burned instead of failing fast", elapsed)
	}
	if !strings.Contains(err.Error(), "Could not resolve hostname") {
		t.Fatalf("WaitForSSH(missing alias) error = %q, want the unresolvable-hostname detail", err)
	}
}
