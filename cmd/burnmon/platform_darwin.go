//go:build darwin

package main

import (
	"log"
	"path/filepath"

	"burnmon/internal/instance"
)

func alreadyRunning() bool {
	held, err := instance.Acquire(filepath.Join(appDataDir(), "burnmon.lock"))
	if err != nil {
		log.Println("single-instance lock failed:", err)
		return false
	}
	return held
}

// bringExistingToFront only logs: macOS gives no documented way to raise
// another process's window from here.
func bringExistingToFront() {
	log.Println("burnmon is already running; not raising its window")
}

func runWithoutWindow(a *app) {
	log.Println("could not create the window")
}
