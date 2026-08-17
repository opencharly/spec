// spec/container/transfer.go — the container-engine image-transfer host primitive,
// RELOCATED from sdk/kit by #55 coneC (charly/ off sdk/kit): it pipes an image from
// one engine's local store to another via `save | load` (os/exec), the fabric twin of
// the LocalImageExists probe that already lives in this slice. sdk/kit re-exports it
// (var TransferImage = container.TransferImage) so every existing kit.TransferImage
// call site (candy/plugin-build, candy/plugin-box, candy/plugin-deploy-pod, …) is
// unchanged; charly core inlines from here. Co-located with EngineBinary (same package)
// and the LocalImageExists family this transfer path complements.
package container

import (
	"fmt"
	"os"
	"os/exec"
)

// StreamLoad pipes a `save` command's stdout straight into a `load` command's
// stdin — an image transfer with NO intermediate tarball on either side. It is
// the ONE streaming primitive behind every image-delivery path in the tree:
// TransferImage (engine→engine on one host), kit.TransferToRootful
// (rootless→rootful), the VM host→guest cp-box, and the container-venue
// host→nested-store `charly box load`. Each caller supplies its own pair of
// commands; the only thing they ever differed in was how the load side is
// reached, so that — and nothing else — stays at the call site.
//
// A tarball is not merely wasteful here: a multi-GB image cannot land in a
// size-limited tmpfs /tmp, which is what the destination of several of these
// paths actually has.
//
// Both commands' stderr is routed to the caller's stderr so a failure is
// visible rather than swallowed, and the load side's stdout goes there too —
// `podman load` writes its "Loaded image:" line to stdout, which is diagnostic
// output, not a value any caller parses.
//
// NOTE for callers: `podman load` can exit 0 on a TRUNCATED stream, registering
// an image whose overlay layers are incomplete. A green return from StreamLoad
// is therefore NOT proof of an intact image; a caller that cares must probe the
// loaded image afterwards. deploykit's venue transfer does exactly that.
func StreamLoad(save, load *exec.Cmd) error {
	pipe, err := save.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}
	load.Stdin = pipe
	load.Stdout = os.Stderr
	load.Stderr = os.Stderr
	save.Stderr = os.Stderr

	if err := load.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", load.Path, err)
	}
	if err := save.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", save.Path, err)
	}
	saveErr := save.Wait()
	loadErr := load.Wait()
	if saveErr != nil {
		return fmt.Errorf("%s save failed: %w", save.Path, saveErr)
	}
	if loadErr != nil {
		return fmt.Errorf("%s load failed: %w", load.Path, loadErr)
	}
	return nil
}

// TransferImage pipes an image from one engine to another via save | load.
func TransferImage(srcEngine, dstEngine, imageRef string) error {
	srcBinary := EngineBinary(srcEngine)
	dstBinary := EngineBinary(dstEngine)

	fmt.Fprintf(os.Stderr, "Transferring %s from %s to %s\n", imageRef, srcEngine, dstEngine)

	if err := StreamLoad(
		exec.Command(srcBinary, "save", imageRef),
		exec.Command(dstBinary, "load"),
	); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Transferred %s to %s\n", imageRef, dstEngine)
	return nil
}
