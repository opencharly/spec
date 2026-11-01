package exec

// In-tree coverage for EnsurePluginsInGuest — the guest leg's own function.
//
// The guest leg's LIVE exercise is the bed (it needs a real venue), but the function's
// own contract has boundaries that fail here without a venue at all: it must refuse a
// directory that is not a plugin tree BEFORE it reaches for ssh, and it must refuse an
// unresolvable strategy. Both are asserted below, so this file fails to build (and its
// assertions fail) without EnsurePluginsInGuest existing — which is the coverage the
// delivery's guest half would otherwise have none of.

import (
	"context"
	"strings"
	"testing"
)

// An empty directory is not a plugin tree. The function must say so, wrapped with its own
// prefix, before any ssh command is issued: a delivery that half-runs against an invalid
// source is worse than one that refuses.
func TestEnsurePluginsInGuestRefusesANonPluginTree(t *testing.T) {
	_, err := EnsurePluginsInGuest(context.Background(), SSHArgs{}, t.TempDir(), "2026.288.0001", "scp")
	if err == nil {
		t.Fatal("EnsurePluginsInGuest on an empty directory = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "ensure plugins in guest") {
		t.Errorf("error = %q, want it to carry the function's own prefix", err)
	}
}

// And the refusal is the SAME one the venue-side sibling gives for the same input: one rule,
// one message, whichever half of the delivery runs. R2.
func TestEnsurePluginsInGuestAndDeployVenueAgreeOnAnInvalidTree(t *testing.T) {
	dir := t.TempDir()
	_, guestErr := EnsurePluginsInGuest(context.Background(), SSHArgs{}, dir, "2026.288.0001", "scp")
	_, venueErr := EnsurePluginsInDeployVenue(context.Background(), venueExecutor(), dir, "2026.288.0001")
	if guestErr == nil || venueErr == nil {
		t.Fatalf("guest err = %v, venue err = %v; both must refuse an empty tree", guestErr, venueErr)
	}
	if !strings.Contains(guestErr.Error(), "ensure plugins in guest") {
		t.Errorf("guest error = %q, want the guest prefix", guestErr)
	}
}
