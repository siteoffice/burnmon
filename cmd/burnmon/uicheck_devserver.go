package main

import "burnmon/internal/winbind"

// startUICheckServer is dev-only: burnmon.exe's eval channel for tools\uicheck,
// on when BURNMON_UICHECK is set, on 127.0.0.1:9333, reporting through the
// page function bmUICheckResult. The server itself lives in
// burnmon/internal/winbind (StartUICheckServer), shared with burnmon-dev.
func startUICheckServer(w winbind.Window) {
	winbind.StartUICheckServer(w, winbind.UICheckOptions{
		Env:     "BURNMON_UICHECK",
		Addr:    "127.0.0.1:9333",
		Binding: "bmUICheckResult",
	})
}
