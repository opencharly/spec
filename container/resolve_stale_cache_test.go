package container

import (
	"errors"
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestResolveLocalImage_RetriesOnStaleCacheMiss is the regression guard for a group bed
// with two image-backed members being unable to check its second member.
//
// ListLocalImages serves a 5-minute persistent cache. The bed phase order is: build A,
// check A, build B, check B. Checking A repopulates the cache from a snapshot taken BEFORE
// B was tagged, so checking B — a separate process, and a cache HIT — could not see its own
// freshly built image and the bed died with `image "<box>:<tag>" is not available locally`.
// Observed deterministically on consecutive runs of check-punktfunk-fleet: the passing
// member logged a cache miss, the failing one logged none.
//
// A stale HIT is harmless (it names an image that exists); a stale MISS is not. So a miss
// must be confirmed against a fresh list before it is believed.
func TestResolveLocalImage_RetriesOnStaleCacheMiss(t *testing.T) {
	origList := ListLocalImages
	origExists := LocalImageExists
	defer func() {
		ListLocalImages = origList
		LocalImageExists = origExists
	}()
	LocalImageExists = func(engine, ref string) bool { return true }

	const ref = "ghcr.io/opencharly/punktfunk-host:check-punktfunk-fleet-2026.242.1217"
	fresh := []LocalImageInfo{{
		ID:     "sha256:deadbeef",
		Names:  []string{ref},
		Labels: map[string]string{spec.LabelBox: "punktfunk-host", spec.LabelVersion: "2026.242.1217"},
	}}

	calls := 0
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		calls++
		if calls == 1 {
			return nil, nil // the stale snapshot: taken before this image was tagged
		}
		return fresh, nil // reality
	}

	got, err := ResolveLocalImage("podman", "punktfunk-host:check-punktfunk-fleet-2026.242.1217")
	if err != nil {
		t.Fatalf("a stale-cache miss must be re-checked against a fresh list, got: %v", err)
	}
	if got.Ref != ref {
		t.Errorf("resolved ref = %q, want %q", got.Ref, ref)
	}
	if calls != 2 {
		t.Errorf("expected exactly one re-fetch after the miss, got %d ListLocalImages calls", calls)
	}
}

// A genuine absence must still be an absence: the retry re-checks reality, it does not
// invent an image. Without this, "confirm the miss" could quietly become "retry until
// something turns up".
func TestResolveLocalImage_GenuineAbsenceStillFails(t *testing.T) {
	origList := ListLocalImages
	defer func() { ListLocalImages = origList }()

	calls := 0
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		calls++
		return nil, nil // the image really is not there, on either pass
	}

	_, err := ResolveLocalImage("podman", "no-such-box:2026.001.0000")
	if !errors.Is(err, spec.ErrImageNotLocal) {
		t.Fatalf("a real absence must report ErrImageNotLocal, got: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected the miss to be confirmed exactly once, got %d calls", calls)
	}
}

// The fast path must stay fast: a cache HIT resolves without a second enumeration, because
// a hit cannot be wrong in the direction that matters.
func TestResolveLocalImage_CacheHitDoesNotRefetch(t *testing.T) {
	origList := ListLocalImages
	origExists := LocalImageExists
	defer func() {
		ListLocalImages = origList
		LocalImageExists = origExists
	}()
	LocalImageExists = func(engine, ref string) bool { return true }

	const ref = "ghcr.io/opencharly/punktfunk-client:check-punktfunk-fleet-2026.242.1217"
	calls := 0
	ListLocalImages = func(engine string) ([]LocalImageInfo, error) {
		calls++
		return []LocalImageInfo{{
			ID:     "sha256:cafe",
			Names:  []string{ref},
			Labels: map[string]string{spec.LabelBox: "punktfunk-client", spec.LabelVersion: "2026.242.1217"},
		}}, nil
	}

	if _, err := ResolveLocalImage("podman", "punktfunk-client:check-punktfunk-fleet-2026.242.1217"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if calls != 1 {
		t.Errorf("a cache hit must not trigger a re-fetch, got %d calls", calls)
	}
}
