//go:build windows

package main

import (
	"golang.org/x/sys/windows"

	"burnmon/internal/instance"
)

// ---------------------------------------------------------------------------
// Single instance, the shared recipe in internal\instance with this app's own
// mutex and window title, so BurnMon and BurnMon Dev never mistake each other
// for a second instance of themselves.
// ---------------------------------------------------------------------------

// user32 and procSetForegroundWindow stay here because front_windows.go and
// windowstate.go use them.
var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

func alreadyRunning() bool { return instance.AlreadyRunning(mutexName) }

func bringExistingToFront() { instance.BringToFront(windowTitle) }
