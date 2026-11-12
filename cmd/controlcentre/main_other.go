//go:build !windows && !darwin

// Command controlcentre builds for Windows (WebView2) and macOS (WKWebView,
// cgo, on a Mac). This stub only keeps `go build ./...` and `go vet ./...`
// runnable on Linux.
package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintf(os.Stderr, "controlcentre has no Linux build; this is a %s/%s build with nothing to run.\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
