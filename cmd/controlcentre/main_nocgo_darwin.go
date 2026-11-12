//go:build darwin && !cgo

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "controlcentre on macOS needs a cgo build on a Mac: CGO_ENABLED=1 go build ./cmd/controlcentre")
	os.Exit(1)
}
