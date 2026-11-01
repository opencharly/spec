package exec

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// opencharly/spec#209's headline behaviour, made assertable: an OBSERVED cloud-init failure is warned
// about, and everything else (a timeout, a poll failure) stays silent — which is what the seam's own
// doc has always claimed for timeouts and never for errors.
func TestCloudInitWarning(t *testing.T) {
	if msg, ok := cloudInitWarning(fmt.Errorf("wrapped: %w", ErrCloudInitFailed)); !ok || msg == "" {
		t.Errorf("cloudInitWarning(ErrCloudInitFailed) = (%q, %v), want a message and true", msg, ok)
	}
	for _, err := range []error{nil, context.DeadlineExceeded, errors.New("ssh dropped")} {
		if msg, ok := cloudInitWarning(err); ok || msg != "" {
			t.Errorf("cloudInitWarning(%v) = (%q, %v), want silence", err, msg, ok)
		}
	}
}
