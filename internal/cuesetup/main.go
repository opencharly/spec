// Command cuesetup provisions the pinned `cue` CLI into a directory. It is the
// task-facing entry point to cuetoolchain.Ensure, so `charly task bootstrap-cue` and
// any other consumer of the pinned CLI cannot disagree about which toolchain they run,
// and so the pin, its URL and its checksum exist in exactly one place (cuetoolchain)
// instead of once in Go and once in shell. It is NOT on the generation leg: the
// schema→Go params pipeline composes and generates in-process (no CLI).
package main

import (
	"fmt"
	"os"

	"github.com/opencharly/spec/cuetoolchain"
)

func main() {
	dir := "bin"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	bin, err := cuetoolchain.Ensure(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cuesetup: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("cuesetup: %s is %s (already present or provisioned)\n", bin, cuetoolchain.Pin())
}
