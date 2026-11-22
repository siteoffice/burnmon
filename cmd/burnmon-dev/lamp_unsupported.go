package main

import (
	"fmt"
	"io"
)

// lampUnsupported is the whole `lamp` subcommand on a platform that has no
// lamp window: Linux, and macOS without cgo. It carries no build tag so a
// test can run it on Windows. Every argument, --poses and --card included,
// gets the same sentence and exit code 2. The sentence names the builds that
// do have the lamp: the Windows build and the Mac app, which is built with cgo.
func lampUnsupported(w io.Writer) int {
	fmt.Fprintln(w, "This build has no lamp window. The lamp runs in the Windows build and in the Mac app (the build with cgo).")
	return 2
}
