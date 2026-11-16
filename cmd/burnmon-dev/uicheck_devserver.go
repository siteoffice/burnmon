//go:build windows || darwin

package main

import "burnmon/internal/winbind"

// startUICheckServer is dev-only: burnmon-dev.exe's eval channel for
// tools\uicheck, one exe and one port over from cmd\burnmon\uicheck_devserver.go:
// BURNMON_DEV_UICHECK (not BURNMON_UICHECK) gates it, it listens on
// 127.0.0.1:9334 (not 9333) and reports through bdevUICheckResult. A distinct
// env var and port, because both exes run at once as the normal case and
// tools\uicheck must be able to drive either one. page builds the document the
// window starts with; a request {"reload":true} loads it again. The server
// itself lives in burnmon/internal/winbind (StartUICheckServer).
func startUICheckServer(w winbind.Window, page func() string) {
	winbind.StartUICheckServer(w, winbind.UICheckOptions{
		Env:     "BURNMON_DEV_UICHECK",
		Addr:    "127.0.0.1:9334",
		Binding: "bdevUICheckResult",
		Page:    page,
	})
}
