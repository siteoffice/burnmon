//go:build !windows && !darwin

// Command burnmon-dev builds for Windows (WebView2) and macOS (WKWebView, cgo,
// on a Mac). Linux has no BurnMon Dev build yet (roadmap row 17); this stub only
// keeps `go build ./...`/`go vet ./...` runnable on Linux.
package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintf(os.Stderr, "burnmon-dev has no Linux build yet; this is a %s/%s build with nothing to run.\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
