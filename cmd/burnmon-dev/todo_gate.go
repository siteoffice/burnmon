//go:build windows || darwin

package main

import "os"

// todoEnabledForRun gates Microsoft To Do behind whether this process is
// running under tools\uicheck (BURNMON_DEV_UICHECK set, uicheck_devserver.go's
// own gate). WS2 follow-up (2026-09-28), item 2: a uicheck run drives a real
// signed-in session (Wilco stays signed in day to day), so without this an
// automated sweep would call the live Microsoft Graph API and screenshot
// real task titles into testdata\uicheck\out. Forcing the config's own
// enabled flag off for the run's whole lifetime, rather than feeding fake
// rows, means the sign-in binding (bdevTodoLogin) and the status/refresh
// loop (startSlowRefreshers) never touch Graph at all - the panel just
// renders its ordinary "not enabled" state, never a live or fake row.
func todoEnabledForRun(configEnabled, uicheckActive bool) bool {
	if uicheckActive {
		return false
	}
	return configEnabled
}

// uicheckActive reports whether BURNMON_DEV_UICHECK is set, the same env
// var uicheck_devserver.go itself gates on.
func uicheckActive() bool {
	return os.Getenv("BURNMON_DEV_UICHECK") != ""
}
