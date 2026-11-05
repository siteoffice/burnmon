//go:build darwin

package main

import (
	"log"
	"os/exec"
	"path/filepath"

	"burnmon/internal/instance"
)

func alreadyRunning() bool {
	held, err := instance.Acquire(filepath.Join(appDataDir(), "burnmon-dev.lock"))
	if err != nil {
		log.Println("single-instance lock failed:", err)
		return false
	}
	return held
}

// bringExistingToFront only logs: macOS gives no documented way to raise
// another process's window from here.
func bringExistingToFront() {
	log.Println("burnmon-dev is already running; not raising its window")
}

// startBrowser opens rawURL in the default browser.
func startBrowser(rawURL string) error {
	return exec.Command("/usr/bin/open", rawURL).Start()
}

// openItemFile opens path with the default app. itemFileAllowed still refuses
// non-drive paths, so item links stay closed on macOS.
var openItemFile = func(path string) error {
	return exec.Command("/usr/bin/open", path).Start()
}

// readClocks returns a zero pair, so sleptBetween is 0: no documented macOS
// source for time asleep.
func readClocks() clockPair { return clockPair{} }

func powerState() string { return "not on macOS" }

// windowHooks has nothing to hold: webview_go has no fullscreen call.
type windowHooks struct{}

func (windowHooks) toggleFullscreen() {}
func (windowHooks) exitFullscreen()   {}

// startToasts starts the notifier lazily, on the first banner.
func startToasts(snapPath, setDir string, backfilled <-chan struct{}, h windowHooks) (stop func()) {
	return startLazyToasts(snapPath, setDir, backfilled, nil)
}

// platformNotOn labels System zone readings macOS has no documented source for.
const platformNotOn = "macOS"
