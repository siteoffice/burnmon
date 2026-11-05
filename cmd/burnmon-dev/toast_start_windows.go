//go:build windows

package main

import (
	"log"

	"burnmon/internal/toast"
)

// startToasts (B8): a Windows notification when something enters Waits on you.
// The notifier has its own window on its own thread and the watch its own
// goroutine, which reads toast_enabled from burnmon-dev.json on every new
// snapshot and stays quiet until the startup backfill is done. On the UI
// thread only bdevSetToast runs, and it writes the small json file like
// bdevSetStationTheme does. The returned stop closes the notifier.
func startToasts(snapPath, setDir string, backfilled <-chan struct{}, h windowHooks) (stop func()) {
	hwnd := h.hwnd
	n, err := toast.Start("BurnMon Dev", func() { bringToFront(hwnd) })
	if err != nil {
		log.Println("toasts off for this run:", err)
		return func() {}
	}
	startToastWatch(snapPath, setDir, backfilled, n.Show)
	return n.Close
}
