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

// TransferImage pipes an image from one engine to another via save | load.
func TransferImage(srcEngine, dstEngine, imageRef string) error {
	srcBinary := EngineBinary(srcEngine)
	dstBinary := EngineBinary(dstEngine)

	fmt.Fprintf(os.Stderr, "Transferring %s from %s to %s\n", imageRef, srcEngine, dstEngine)

	save := exec.Command(srcBinary, "save", imageRef)
	load := exec.Command(dstBinary, "load")

	pipe, err := save.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}
	load.Stdin = pipe
	load.Stderr = os.Stderr

	if err := load.Start(); err != nil {
		return fmt.Errorf("starting %s load: %w", dstBinary, err)
	}
	if err := save.Run(); err != nil {
		return fmt.Errorf("%s save failed: %w", srcBinary, err)
	}
	if err := load.Wait(); err != nil {
		return fmt.Errorf("%s load failed: %w", dstBinary, err)
	}

	fmt.Fprintf(os.Stderr, "Transferred %s to %s\n", imageRef, dstEngine)
	return nil
}