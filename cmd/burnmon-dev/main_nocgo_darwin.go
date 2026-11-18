//go:build darwin && !cgo

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "lamp" {
		os.Exit(lampUnsupported(os.Stderr))
	}
	fmt.Fprintln(os.Stderr, "burnmon-dev on macOS needs a cgo build on a Mac: CGO_ENABLED=1 go build ./cmd/burnmon-dev")
	os.Exit(1)
}
