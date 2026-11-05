//go:build darwin && !cgo

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "burnmon-dev on macOS needs a cgo build on a Mac: CGO_ENABLED=1 go build ./cmd/burnmon-dev")
	os.Exit(1)
}
