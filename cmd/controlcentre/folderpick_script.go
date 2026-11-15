//go:build windows || darwin

package main

import (
	"os"
	"strings"
)

// appleChooseFolder is the AppleScript the macOS picker runs: choose folder,
// opened in start when start is a folder. It lives in a file both builds
// compile, so the Windows tests check it too.
func appleChooseFolder(start string) string {
	s := `POSIX path of (choose folder with prompt "Pick the Control Centre root"`
	if fi, err := os.Stat(start); err == nil && fi.IsDir() {
		q := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(start)
		s += ` default location (POSIX file "` + q + `")`
	}
	return s + ")"
}

// osascriptPath trims osascript's output to a folder path: no line end and
// no trailing slash except on "/".
func osascriptPath(out string) string {
	p := strings.TrimRight(out, "\r\n")
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}
