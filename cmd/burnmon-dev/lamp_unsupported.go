package main

import (
	"fmt"
	"io"
)

// lampUnsupported is the whole `lamp` subcommand on a platform that has no
// lamp window: Linux, and macOS without cgo. It carries no build tag so a
// test can run it on Windows. Every argument, --poses and --card included,
// gets the same sentence and exit code 2.
func lampUnsupported(w io.Writer) int {
	fmt.Fprintln(w, "The lamp runs on Windows only in this version.")
	return 2
}
