//go:build windows || darwin

package main

import "burnmon/internal/winbind"

// startUICheckServer is dev-only: controlcentre.exe's eval channel for
// tools\uicheck (the gitpage group), one exe and one port over from
// cmd\burnmon-dev\uicheck_devserver.go. CONTROLCENTRE_UICHECK gates it, it
// listens on 127.0.0.1:9335 (burnmon.exe has 9333, burnmon-dev.exe 9334) and
// reports through ccUICheckResult. Release builds and the README never set the
// variable, so no port opens for a user. The server itself lives in
// burnmon/internal/winbind (StartUICheckServer).
func startUICheckServer(w winbind.Window) {
	winbind.StartUICheckServer(w, winbind.UICheckOptions{
		Env:     "CONTROLCENTRE_UICHECK",
		Addr:    "127.0.0.1:9335",
		Binding: "ccUICheckResult",
	})
}
